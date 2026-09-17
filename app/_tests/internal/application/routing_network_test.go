package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-proxy/internal/config"
	"go-proxy/internal/network"
	"go-proxy/internal/store"
)

func routingApplicationFixture(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	for _, item := range []struct {
		target *string
		name   string
	}{
		{&config.SingBoxConfig, "sing-box.json"}, {&config.UserMetaFile, "users.json"}, {&config.UserRouteFile, "routes.json"}, {&config.UserTemplateFile, "templates.json"}, {&config.FirewallConfigFile, "firewall.json"}, {&config.SnellConfigFile, "snell.conf"}, {&config.SingBoxBin, "sing-box"}, {&config.DomainFile, "domain"}, {&config.CaddyFile, "Caddyfile"},
	} {
		saved := *item.target
		*item.target = filepath.Join(dir, item.name)
		t.Cleanup(func() { *item.target = saved })
	}
	for path, data := range map[string]string{
		config.SingBoxConfig: `{"dns":{"strategy":"prefer_ipv6"},"outbounds":[{"type":"direct","tag":"direct"}]}`,
		config.UserMetaFile:  `{"schema":3,"groups":{"all":["alice","bob"]}}`,
		config.SingBoxBin:    "#!/bin/sh\nexit 0\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	a := New(func(string) {})
	a.LockDir = filepath.Join(dir, "locks")
	a.RequireRoot = false
	if _, err := a.Operation(context.Background(), func() (Result, error) { return Result{}, nil }); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRoutingOperationsPersistStrategyAndRejectStaleIndexes(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if result, err := a.RoutingSet(ctx, "alice", []string{"openai", "google"}, "direct"); err != nil || !result.Changed {
		t.Fatalf("set: %#v %v", result, err)
	}
	if result, err := a.RoutingSet(ctx, "alice", []string{"openai", "google"}, "direct"); err != nil || result.Changed {
		t.Fatalf("repeat set: %#v %v", result, err)
	}
	if _, err := a.RoutingRules(ctx, "alice", []int{99}, "", true); err == nil {
		t.Fatal("stale index accepted")
	} else {
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "invalid_argument" {
			t.Fatalf("wrong error: %v", err)
		}
	}
	if _, err := a.RoutingRules(ctx, "alice", []int{1}, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSyncDNS(ctx); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.SingBox.DNS.Strategy != "prefer_ipv6" {
		t.Fatal("strategy was reset")
	}
	if len(saved.UserRoutes) != 1 {
		t.Fatalf("unexpected routes: %#v", saved.UserRoutes)
	}
	for _, rule := range saved.SingBox.DNS.Rules {
		if len(rule.AuthUser) > 0 && rule.Strategy != "prefer_ipv6" {
			t.Fatal("compiled DNS strategy was reset")
		}
	}
}

func TestChainQueriesRedactCredentialsAndReferencedRemovalFails(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "relay-a", "192.0.2.10", 1080, "secret-user", "secret-password"); err != nil {
		t.Fatal(err)
	}
	result, err := a.RoutingChains(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-") {
		t.Fatal("chain query disclosed credentials")
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai"}, "relay-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingChainRemove(ctx, "relay-a"); err == nil {
		t.Fatal("referenced chain removed")
	}
	if _, err := a.RoutingClear(ctx, "alice", false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingChainRemove(ctx, "relay-a"); err != nil {
		t.Fatal(err)
	}
	if result, err := a.RoutingChainRemove(ctx, "relay-a"); err != nil || result.Changed {
		t.Fatalf("repeated chain removal: %#v %v", result, err)
	}
}

func TestCustomFirewallPortsArePersistedWithoutInitializingNft(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if result, err := a.NetworkFirewallPort(ctx, 18443, "both", false); err != nil || !result.Changed {
		t.Fatalf("add custom port: %#v %v", result, err)
	}
	if result, err := a.NetworkFirewallPort(ctx, 18443, "both", false); err != nil || result.Changed {
		t.Fatalf("repeat custom port: %#v %v", result, err)
	}
	if _, err := a.NetworkFirewallPort(ctx, 18443, "tcp", true); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Firewall.Ports) != 1 || saved.Firewall.Ports[0].Proto != "udp" {
		t.Fatalf("wrong remaining custom ports: %#v", saved.Firewall.Ports)
	}
}

func TestNetworkStatusHonorsLongerObservationDeadline(t *testing.T) {
	a := routingApplicationFixture(t)
	sshd := filepath.Join(os.Getenv("PATH"), "sshd")
	if err := os.WriteFile(sshd, []byte("#!/bin/sh\n/bin/sleep 2.1\nprintf 'port 2222\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	result, err := a.NetworkStatus(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	found := false
	for _, port := range data["desired_ports"].([]network.FirewallPortSpec) {
		if port.Proto == "tcp" && port.Port == 2222 {
			found = true
		}
	}
	if !found {
		t.Fatal("explicit longer deadline was cut short by a fixed observation timeout")
	}
	info := data["network"].(network.Observation)
	if info.IPv4.State != "not_checked" || info.IPv6.State != "not_checked" {
		t.Fatal("local network status performed a public probe")
	}
}
