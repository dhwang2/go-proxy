package network

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

func TestDesiredFirewallPortsOpenCaddysPorts(t *testing.T) {
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
		SingBox:  &store.SingBoxConfig{},
		UserMeta: store.NewUserManagement(),
	}

	specs, err := DesiredFirewallPorts(context.Background(), s)
	if err != nil {
		t.Fatalf("DesiredFirewallPorts error: %v", err)
	}

	if !hasCaddyPorts(specs, 80, store.DefaultCaddyPort) {
		t.Fatalf("expected tcp/80 and the default caddy port, got %#v", specs)
	}

	// Once caddy's site is moved, the firewall follows the Caddyfile.
	if err := os.WriteFile(config.CaddyFile, []byte("{\n}\n\nexample.com:443 {\n    file_server\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, err = DesiredFirewallPorts(context.Background(), s)
	if err != nil {
		t.Fatalf("DesiredFirewallPorts error: %v", err)
	}
	if !hasCaddyPorts(specs, 80, 443) || hasCaddyPorts(specs, store.DefaultCaddyPort) {
		t.Fatalf("expected caddy on tcp/80 and tcp/443 only, got %#v", specs)
	}
}

func hasCaddyPorts(specs []FirewallPortSpec, ports ...int) bool {
	for _, port := range ports {
		found := false
		for _, spec := range specs {
			if spec.Proto == "tcp" && spec.Port == port && slices.Contains(spec.Sources, "caddy") {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestDesiredFirewallPortsIncludesCustomPorts(t *testing.T) {
	s := &store.Store{
		SingBox:  &store.SingBoxConfig{},
		UserMeta: store.NewUserManagement(),
		Firewall: &store.FirewallConfig{
			Ports: []store.FirewallPort{
				{Proto: "udp", Port: 5353},
				{Proto: "tcp", Port: 9443},
			},
		},
	}

	specs, err := DesiredFirewallPorts(context.Background(), s)
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
	ports, err := DesiredFirewallPorts(context.Background(), s)
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
	firewallBootFixture(t, dir)
	rulesPath := filepath.Join(dir, "rules.nft")
	script := "#!/bin/sh\ncase \"$1\" in\n-j) echo '{\"nftables\":[{\"table\":{\"family\":\"inet\",\"name\":\"proxy_firewall\"}}]}';;\nlist) exit 0;;\n-f) cp \"$2\" \"$GPROXY_TEST_NFT_RULES\";;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GPROXY_TEST_NFT_RULES", rulesPath)
	for _, udpPorts := range [][]int{nil, {27200}} {
		if err := nftApplyPorts(context.Background(), []int{22, 443}, udpPorts); err != nil {
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
		if !strings.HasPrefix(rules, "table inet proxy_firewall\ndelete table inet proxy_firewall\n") || !strings.Contains(rules, "tcp dport { 22, 443 } accept") {
			t.Fatal("convergence must replace the old table and retain SSH/proxy ports")
		}
	}
}

// firewallBootFixture points the boot files into dir and puts a systemctl
// there that logs its arguments and keeps "enabled" as a file.
func firewallBootFixture(t *testing.T, dir string) (calls string) {
	t.Helper()
	saved := []string{FirewallRulesPath, FirewallUnitPath}
	FirewallRulesPath = filepath.Join(dir, "conf", "proxy-firewall.nft")
	FirewallUnitPath = filepath.Join(dir, "units", "proxy-firewall.service")
	t.Cleanup(func() { FirewallRulesPath, FirewallUnitPath = saved[0], saved[1] })
	calls = filepath.Join(dir, "systemctl.log")
	enabled := filepath.Join(dir, "enabled")
	script := "#!/bin/sh\necho \"$*\" >> '" + calls + "'\ncase \"$1\" in\n" +
		"is-enabled) test -f '" + enabled + "'; exit $?;;\n" +
		"enable) : > '" + enabled + "';;\n" +
		"disable) /bin/rm -f '" + enabled + "';;\n" +
		"esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return calls
}

// fakeNft loads a ruleset by keeping a copy of it, lists it back, and refuses
// every ruleset while dir/refuse exists.
func fakeNft(t *testing.T, dir string) (loaded string) {
	t.Helper()
	loaded = filepath.Join(dir, "loaded.nft")
	script := "#!/bin/sh\ncase \"$1\" in\n" +
		"-j) if [ -f '" + loaded + "' ]; then echo '{\"nftables\":[{\"table\":{\"family\":\"inet\",\"name\":\"proxy_firewall\"}}]}'; else echo '{\"nftables\":[]}'; fi;;\n" +
		"-f) [ -f '" + filepath.Join(dir, "refuse") + "' ] && exit 1; /bin/cat \"$2\" > '" + loaded + "';;\n" +
		"list) /bin/cat '" + loaded + "';;\n" +
		"delete) /bin/rm -f '" + loaded + "';;\n" +
		"*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return loaded
}

// A reboot loads what apply loaded: apply writes gproxy's ruleset file and
// enables its unit, every later apply rewrites the file, a ruleset nft refuses
// leaves the boot copy as it was, and release removes both. The host's own
// /etc/nftables.conf is never involved, and nothing flushes the ruleset.
func TestFirewallApplyKeepsTheTableAtBoot(t *testing.T) {
	dir := t.TempDir()
	calls := firewallBootFixture(t, dir)
	loaded := fakeNft(t, dir)
	t.Setenv("PATH", dir)
	ctx := context.Background()
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	if err := nftApplyPorts(ctx, []int{22, 443}, []int{27200}); err != nil {
		t.Fatal(err)
	}
	rules := read(FirewallRulesPath)
	if rules != read(loaded) {
		t.Fatal("the boot ruleset differs from the one loaded")
	}
	if strings.Contains(rules, "flush") || !strings.Contains(rules, "tcp dport { 22, 443 } accept") || !strings.Contains(rules, "udp dport { 27200 } accept") {
		t.Fatalf("boot ruleset:\n%s", rules)
	}
	unit := read(FirewallUnitPath)
	for _, want := range []string{
		"ExecStart=" + filepath.Join(dir, "nft") + " -f " + FirewallRulesPath + "\n",
		"After=nftables.service\n",
		"Before=network-pre.target network-online.target sing-box.service snell-v6.service caddy-sub.service",
		"DefaultDependencies=no\n",
		"Type=oneshot\n",
		"WantedBy=sysinit.target\n",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit lacks %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "ExecStop") {
		t.Fatal("stopping the unit must not touch the table")
	}
	if !FirewallKeptAtBoot(ctx) {
		t.Fatal("apply did not keep the table at boot")
	}

	// A reconcile rewrites the file; an unchanged unit is not reloaded.
	if err := nftApplyPorts(ctx, []int{22}, nil); err != nil {
		t.Fatal(err)
	}
	if rules := read(FirewallRulesPath); strings.Contains(rules, "443") || strings.Contains(rules, "udp dport { ") {
		t.Fatalf("reconcile left the boot ruleset stale:\n%s", rules)
	}
	if log := read(calls); strings.Count(log, "daemon-reload") != 1 || strings.Count(log, "enable --quiet "+firewallUnitName) != 2 {
		t.Fatalf("systemctl calls:\n%s", log)
	}

	// A ruleset nft refuses does not replace the one a reboot restores.
	before := read(FirewallRulesPath)
	if err := os.WriteFile(filepath.Join(dir, "refuse"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := nftApplyPorts(ctx, []int{22, 8443}, nil); err == nil {
		t.Fatal("a refused ruleset was reported as applied")
	}
	if read(FirewallRulesPath) != before {
		t.Fatal("a refused ruleset replaced the boot copy")
	}
	os.Remove(filepath.Join(dir, "refuse"))

	// The table gone but the boot files there: still applied, so the next
	// change reconciles both rather than leaving the file behind.
	os.Remove(loaded)
	if applied, err := FirewallApplied(ctx); err != nil || !applied {
		t.Fatalf("applied = %v, %v", applied, err)
	}

	if err := RemoveFirewallRules(ctx); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{FirewallRulesPath, FirewallUnitPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("release left %s", path)
		}
	}
	if !strings.Contains(read(calls), "disable --now "+firewallUnitName) || FirewallKeptAtBoot(ctx) {
		t.Fatalf("release did not disable the unit:\n%s", read(calls))
	}
	if applied, err := FirewallApplied(ctx); err != nil || applied {
		t.Fatalf("released firewall still applied: %v, %v", applied, err)
	}
}

// Status reads the boot files without changing them.
func TestFirewallStatusSaysWhetherTheTableIsKeptAtBoot(t *testing.T) {
	dir := t.TempDir()
	calls := firewallBootFixture(t, dir)
	fakeNft(t, dir)
	t.Setenv("PATH", dir)
	ctx := context.Background()
	s := &store.Store{SingBox: &store.SingBoxConfig{}, UserMeta: store.NewUserManagement()}

	info, err := FirewallStatus(ctx, s, nil)
	if err != nil || info.Managed || info.Boot || info.BootRules != "" {
		t.Fatalf("unapplied: %+v %v", info, err)
	}
	if err := ApplyFirewallConvergence(ctx, s); err != nil {
		t.Fatal(err)
	}
	info, err = FirewallStatus(ctx, s, nil)
	if err != nil || !info.Managed || !info.Boot || info.BootRules != FirewallRulesPath {
		t.Fatalf("applied: %+v %v", info, err)
	}
	// Disabled from outside gproxy: the files are there, a reboot would not
	// load them.
	if err := os.Remove(filepath.Join(dir, "enabled")); err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(calls)
	info, err = FirewallStatus(ctx, s, nil)
	if err != nil || info.Boot || info.BootRules != FirewallRulesPath {
		t.Fatalf("disabled unit: %+v %v", info, err)
	}
	after, _ := os.ReadFile(calls)
	if extra := strings.TrimPrefix(string(after), string(log)); strings.TrimSpace(extra) != "is-enabled --quiet "+firewallUnitName {
		t.Fatalf("status ran more than a query: %q", extra)
	}
}
