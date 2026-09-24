package network

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFirewallInspectionNeverInstallsTools(t *testing.T) {
	dir := t.TempDir()
	called := filepath.Join(dir, "apt-called")
	script := "#!/bin/sh\ntouch '" + called + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "apt-get"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if managed, err := FirewallManaged(context.Background()); err != nil || managed {
		t.Fatalf("unexpected state: %v %v", managed, err)
	}
	if _, err := os.Stat(called); !os.IsNotExist(err) {
		t.Fatal("inspection ran apt-get")
	}
}

func TestFirewallInspectionReportsCommandFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte("#!/bin/sh\nexit 42\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if _, err := FirewallManaged(context.Background()); err == nil {
		t.Fatal("failed nft query was reported as an absent table")
	}
}

func TestFirewallClearTouchesOnlyManagedTable(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/bin/sh\ncase \"$1\" in\n-j) echo '{\"nftables\":[{\"table\":{\"family\":\"inet\",\"name\":\"proxy_firewall\"}},{\"table\":{\"family\":\"inet\",\"name\":\"other\"}}]}';;\n*) echo \"$*\" >> '" + calls + "' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := RemoveFirewallRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "delete table inet proxy_firewall" {
		t.Fatalf("unexpected writes: %s", raw)
	}
}

func TestLocalObservationLeavesPublicChecksUnrequested(t *testing.T) {
	info, err := Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.IPv4.State != "not_checked" || info.IPv6.State != "not_checked" {
		t.Fatalf("unexpected probes: %#v %#v", info.IPv4, info.IPv6)
	}
	for _, addr := range info.Addresses {
		if addr.Source != "interface" {
			t.Fatalf("local address source mislabeled: %#v", addr)
		}
	}
}

func TestProbeValidatesAddressFamilyAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		response := "2606:4700::1111"
		if r.URL.Path == "/valid" {
			response = "1.1.1.1"
		}
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	if got := probeAddress(context.Background(), "tcp4", server.URL); got.State != "unavailable" {
		t.Fatalf("wrong address family accepted: %#v", got)
	}
	if got := probeAddress(context.Background(), "tcp4", server.URL+"/valid"); got.State != "available" || got.Address != "1.1.1.1" {
		t.Fatalf("valid probe failed: %#v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if got := probeAddress(ctx, "tcp4", server.URL+"/slow"); got.State != "timeout" {
		t.Fatalf("timeout not reported: %#v", got)
	}
	if time.Since(start) > time.Second {
		t.Fatal("probe exceeded its cancellation budget")
	}
}

func TestNetworkCommandCancellationAndOutputLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := runCommand(ctx, "sh", "-c", "sleep 30 & wait"); err == nil {
		t.Fatal("cancelled subprocess succeeded")
	}
	if time.Since(started) > time.Second {
		t.Fatal("subprocess did not stop within the cleanup budget")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := runCommand(ctx, "yes"); err == nil || !strings.Contains(err.Error(), "output exceeds") {
		t.Fatalf("unbounded output was not rejected: %v", err)
	}
}
