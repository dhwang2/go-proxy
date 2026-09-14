package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

func TestDesiredFirewallPortsKeepsACMEPortsWhenDomainFileExists(t *testing.T) {
	dir := t.TempDir()
	prevDomainFile := config.DomainFile
	prevCaddyFile := config.CaddyFile
	config.DomainFile = filepath.Join(dir, ".domain")
	config.CaddyFile = filepath.Join(dir, "Caddyfile")
	t.Cleanup(func() {
		config.DomainFile = prevDomainFile
		config.CaddyFile = prevCaddyFile
	})

	if err := os.WriteFile(config.DomainFile, []byte("example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := &store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}

	specs, err := DesiredFirewallPorts(s)
	if err != nil {
		t.Fatalf("DesiredFirewallPorts error: %v", err)
	}

	var has80 bool
	var has443 bool
	for _, spec := range specs {
		if spec.Proto != "tcp" {
			continue
		}
		if spec.Port == 80 {
			has80 = true
		}
		if spec.Port == 443 {
			has443 = true
		}
	}
	if !has80 || !has443 {
		t.Fatalf("expected tcp/80 and tcp/443 in desired firewall ports, got %#v", specs)
	}
}

func TestDesiredFirewallPortsIncludesCustomPorts(t *testing.T) {
	s := &store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
		Firewall: &store.FirewallConfig{
			Ports: []store.FirewallPort{
				{Proto: "udp", Port: 5353},
				{Proto: "tcp", Port: 9443},
			},
		},
	}

	specs, err := DesiredFirewallPorts(s)
	if err != nil {
		t.Fatalf("DesiredFirewallPorts error: %v", err)
	}

	var hasTCP9443 bool
	var hasUDP5353 bool
	for _, spec := range specs {
		if spec.Proto == "tcp" && spec.Port == 9443 && strings.Join(spec.Sources, ",") == "custom" {
			hasTCP9443 = true
		}
		if spec.Proto == "udp" && spec.Port == 5353 && strings.Join(spec.Sources, ",") == "custom" {
			hasUDP5353 = true
		}
	}
	if !hasTCP9443 || !hasUDP5353 {
		t.Fatalf("custom ports missing from desired firewall ports: %#v", specs)
	}
}

func TestSnellV6FirewallUsesTCPOnly(t *testing.T) {
	s := &store.Store{
		SingBox:   &store.SingBoxConfig{},
		SnellConf: &store.SnellConfig{Listen: "0.0.0.0:18443", PSK: "test-password"},
	}
	ports, err := DesiredFirewallPorts(s)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, port := range ports {
		if port.Port == 18443 {
			found = true
			if port.Proto != "tcp" || strings.Join(port.Sources, ",") != "snell-v6" {
				t.Errorf("unexpected Snell v6 port: %#v", port)
			}
		}
	}
	if !found {
		t.Fatal("missing Snell TCP listener")
	}
}

func TestFirewallConvergencePreservesDHCPv6(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.nft")
	script := "#!/bin/sh\ncase \"$1\" in\nlist) exit 0;;\n-f) cp \"$2\" \"$GPROXY_TEST_NFT_RULES\";;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GPROXY_TEST_NFT_RULES", rulesPath)
	for _, udpPorts := range [][]int{nil, {27200}} {
		if err := nftApplyPorts([]int{22, 443}, udpPorts); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(rulesPath)
		if err != nil {
			t.Fatal(err)
		}
		rules := string(data)
		dhcp := strings.Index(rules, "meta nfproto ipv6 udp sport 547 udp dport 546 accept")
		drop := strings.Index(rules, "counter drop")
		if dhcp < 0 || drop < dhcp {
			t.Fatal("DHCPv6 replies must be accepted before the final drop, independently of proxy UDP ports")
		}
		if !strings.Contains(rules, "delete table inet proxy_firewall") || !strings.Contains(rules, "tcp dport { 22, 443 } accept") {
			t.Fatal("convergence must replace the old table and retain SSH/proxy ports")
		}
	}
}
