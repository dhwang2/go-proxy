package application

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"go-proxy/internal/config"
	"go-proxy/internal/network"
	"go-proxy/internal/protocol"
	"go-proxy/internal/routing"
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
	if _, err := a.RoutingRules(ctx, "alice", []string{"b"}, "", true); err == nil {
		t.Fatal("a preset the user has no rule for was accepted")
	} else {
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "invalid_argument" {
			t.Fatalf("wrong error: %v", err)
		}
	}
	if _, err := a.RoutingRules(ctx, "alice", []string{"1"}, "", true); err != nil {
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
	if _, err := a.RoutingChainAdd(ctx, "relay-a", "192.0.2.10", 1080, "secret-user", "secret-password", ""); err != nil {
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
	// --all is what the caller passed, not something derived from --user being
	// empty: clearing one named user is still an --all request.
	if _, err := a.RoutingClear(ctx, "alice", false); err == nil {
		t.Fatal("clearing without --all was accepted")
	}
	cleared, err := a.RoutingClear(ctx, "alice", true)
	if err != nil {
		t.Fatal(err)
	}
	if fields, _ := cleared.Data.(map[string]any); fields["all"] != true || fields["user"] != "alice" {
		t.Fatalf("cleared one user as %#v", cleared.Data)
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

func TestObservationHonorsAnExplicitLongerDeadline(t *testing.T) {
	a := routingApplicationFixture(t)
	sshd := filepath.Join(os.Getenv("PATH"), "sshd")
	if err := os.WriteFile(sshd, []byte("#!/bin/sh\n/bin/sleep 2.1\nprintf 'port 2222\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	result, err := a.NetworkFirewallStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, port := range result.Data.(network.FirewallInfo).Desired {
		if port.Proto == "tcp" && port.Port == 2222 {
			found = true
		}
	}
	if !found {
		t.Fatal("explicit longer deadline was cut short by a fixed observation timeout")
	}
	// status looks up a public address only for a family whose interfaces
	// carry only private addresses (NAT); a family with a global address, or
	// none, is not checked. Which case applies depends on the host running
	// the test: a CI runner behind NAT probes, a host with public addresses
	// does not.
	status, err := a.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	info := status.Data.(map[string]any)["network"].(network.Observation)
	for family, probe := range map[string]network.Probe{"ipv4": info.IPv4, "ipv6": info.IPv6} {
		natted := info.HasFamily(family) && !info.HasGlobal(family)
		if !natted && probe.State != "not_checked" {
			t.Fatalf("status probed %s, which has a public address or none (%s)", family, probe.State)
		}
		if natted && probe.State == "not_checked" {
			t.Fatalf("status did not look up the public address of NATed %s", family)
		}
	}
}

// A rule names an outbound tag, and a chain names the users that selected it.
// Both listings must answer from the same store, or one command would say a
// chain is unused while the other shows a rule pointing at it.
func TestRuleAndChainListingsAgreeOnWhoUsesWhat(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	for _, chain := range []struct {
		tag  string
		host string
	}{{"relay-a", "192.0.2.10"}, {"relay-6", "2001:db8::2"}} {
		if _, err := a.RoutingChainAdd(ctx, chain.tag, chain.host, 1080, "", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai"}, "relay-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "bob", []string{"google"}, "direct"); err != nil {
		t.Fatal(err)
	}

	listed, err := a.RoutingList(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := listed.Data.(map[string]any)["rules"].([]RouteEntry)
	if !ok || len(entries) != 2 {
		t.Fatalf("rules: %#v", listed.Data)
	}
	byUser := map[string]RouteEntry{}
	for _, entry := range entries {
		if entry.Index != 1 {
			t.Fatalf("index %d for the first rule of %s", entry.Index, entry.User)
		}
		byUser[entry.User] = entry
	}
	if byUser["alice"].Outbound != "relay-a" || byUser["alice"].Address != "192.0.2.10:1080" {
		t.Fatalf("chain rule did not resolve its server: %#v", byUser["alice"])
	}
	// Direct egress is not a server, so there is no address to report.
	if byUser["bob"].Outbound != "direct" || byUser["bob"].Address != "" {
		t.Fatalf("direct rule invented an address: %#v", byUser["bob"])
	}

	chains, err := a.RoutingChains(ctx)
	if err != nil {
		t.Fatal(err)
	}
	views, ok := chains.Data.(map[string]any)["chains"].([]ChainView)
	if !ok || len(views) != 2 {
		t.Fatalf("chains: %#v", chains.Data)
	}
	seen := map[string]ChainView{}
	for _, view := range views {
		seen[view.Tag] = view
	}
	if len(seen["relay-a"].Users) != 1 || seen["relay-a"].Users[0] != "alice" {
		t.Fatalf("chain did not report the rule that selects it: %#v", seen["relay-a"])
	}
	if len(seen["relay-6"].Users) != 0 {
		t.Fatalf("unused chain reported users: %#v", seen["relay-6"])
	}
	// An IPv6 literal has to be bracketed to be a valid host:port.
	if seen["relay-6"].Address != "[2001:db8::2]:1080" {
		t.Fatalf("ipv6 chain address %q is not a usable host:port", seen["relay-6"].Address)
	}
}

// The direct strategy should match what this host actually has: only IPv4 asks
// for A alone, only IPv6 for AAAA alone, and a dual-stack host prefers IPv4
// while still allowing the other. Asking for a family the host cannot use is a
// lookup that can only return an address nothing can connect to.
func TestAutoDirectStrategyFollowsTheHostFamilies(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if !slices.Contains(DirectStrategies(), "auto") {
		t.Fatal("auto is not an accepted strategy")
	}
	result, err := a.RoutingDirect(ctx, "auto", true)
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	chosen, _ := data["strategy"].(string)
	// Whatever this machine has, the answer is one of the three the policy
	// allows, and it is reported as detected rather than as a literal choice.
	if chosen != "ipv4_only" && chosen != "ipv6_only" && chosen != "prefer_ipv4" {
		t.Fatalf("auto chose %q, which is not a family-derived strategy", chosen)
	}
	if data["detected"] != chosen {
		t.Fatalf("auto did not report what it detected: %#v", data)
	}
	// The stored strategy is the concrete one, never the word auto.
	saved, err := a.RoutingDirect(ctx, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.Data.(map[string]any)["strategy"]; got != chosen {
		t.Fatalf("stored strategy %q, want the detected %q", got, chosen)
	}
}

// A resolver named by hostname has to be pinned to an address the chain can
// dial: one reached through an IPv4 endpoint cannot be an IPv6 address.
func TestChainResolverIsPinnedToTheChainsFamily(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "v4", "192.0.2.10", 1080, "", "", "https://dns.quad9.net/dns-query"); err != nil {
		t.Skipf("resolver lookup unavailable in this environment: %v", err)
	}
	result, err := a.RoutingChains(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range result.Data.(map[string]any)["chains"].([]ChainView) {
		if view.Tag != "v4" {
			continue
		}
		if view.DomainStrategy != "ipv4_only" {
			t.Fatalf("chain strategy = %q, want ipv4_only", view.DomainStrategy)
		}
		if !strings.Contains(view.Resolver, "dns.quad9.net") {
			t.Fatalf("resolver lost its certificate name: %q", view.Resolver)
		}
		// The address dialled must be IPv4, whatever families the name has.
		fields := strings.Fields(view.Resolver)
		address := fields[len(fields)-1]
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			t.Fatalf("resolver address %q is not host:port", address)
		}
		if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
			t.Fatalf("an IPv4 chain pinned its resolver to %q", host)
		}
		return
	}
	t.Fatal("chain not listed")
}

// Recompiling a configuration that is already correct produces the same bytes.
// Reporting that as a change tells a caller something happened when nothing
// did, and sync-dns is the one operation whose whole job is to recompile.
func TestSyncDNSReportsChangedOnlyWhenTheConfigurationDiffers(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai"}, "direct"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config.SingBoxConfig)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing to pick up: the rules were compiled by the change above.
	result, err := a.RoutingSyncDNS(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatal("a recompile that changed nothing reported changed")
	}
	after, err := os.ReadFile(config.SingBoxConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a recompile that changed nothing rewrote the configuration")
	}

	// Something to pick up: the compiled rules are edited out from under it,
	// which is the shape of an upgrade that compiles rules differently.
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Store.SingBox.Route.Rules = nil
	snapshot.Store.MarkDirty(store.FileSingBox)
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	result, err = a.RoutingSyncDNS(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatal("a recompile that restored the rules reported no change")
	}
	restored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rule := range restored.SingBox.Route.Rules {
		if len(rule.AuthUser) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("the recompile did not restore the user rules")
	}
}

// The internal spelling of the direct outbound is decorated so it sorts and
// reads distinctly inside sing-box. That is an implementation detail: a caller
// asked for "direct" and every payload has to answer with the name it used.
func TestRoutingPayloadsNameDirectPlainly(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	result, err := a.RoutingSet(ctx, "alice", []string{"openai"}, "direct")
	if err != nil {
		t.Fatal(err)
	}
	if fields, _ := result.Data.(map[string]any); fields["outbound"] != "direct" {
		t.Fatalf("rule creation reported outbound %#v", result.Data)
	}
	for _, payload := range []func() (Result, error){
		func() (Result, error) { return a.RoutingTest(ctx, "alice", "api.openai.com") },
		func() (Result, error) { return a.RoutingList(ctx, "alice") },
	} {
		result, err := payload()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "\U0001f438") {
			t.Fatalf("payload carries the internal direct tag: %s", raw)
		}
		if !strings.Contains(string(raw), `"direct"`) {
			t.Fatalf("payload names no outbound at all: %s", raw)
		}
	}
}

// A user name that no user has came from --user, so it is an invalid argument
// and exits 2. Routing answered not_found and exited 1 while `sub` and
// `user rename` exited 2 for the same mistake.
func TestMissingUserIsOneErrorAcrossRouting(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	calls := map[string]func() (Result, error){
		"rule list":   func() (Result, error) { return a.RoutingList(ctx, "ghost") },
		"rule add":    func() (Result, error) { return a.RoutingSet(ctx, "ghost", []string{"openai"}, "direct") },
		"rule remove": func() (Result, error) { return a.RoutingRules(ctx, "ghost", []string{"1"}, "", true) },
		"rule clear":  func() (Result, error) { return a.RoutingClear(ctx, "ghost", true) },
		"test":        func() (Result, error) { return a.RoutingTest(ctx, "ghost", "example.com") },
	}
	for name, call := range calls {
		_, err := call()
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "invalid_argument" {
			t.Fatalf("%s on a missing user gave %v", name, err)
		}
		if detail.Message != "user not found" {
			t.Fatalf("%s reported %q", name, detail.Message)
		}
	}
}

// A chain a rule selects cannot be removed, so changing where it points used to
// mean sending every rule elsewhere, removing the chain, adding it back and
// sending the rules home again. modify changes it in place, and the rules never
// stop selecting it.
func TestChainModifyKeepsTheRulesThatSelectIt(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "relay-a", "192.0.2.10", 1080, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai"}, "relay-a"); err != nil {
		t.Fatal(err)
	}
	// The chain is now referenced, which is what makes removal impossible.
	if _, err := a.RoutingChainRemove(ctx, "relay-a"); err == nil {
		t.Fatal("a referenced chain was removable")
	}

	result, err := a.RoutingChainModify(ctx, "relay-a", ChainChange{Host: "192.0.2.20", Port: 1081})
	if err != nil || !result.Changed {
		t.Fatalf("modify: %#v %v", result, err)
	}
	view, _ := result.Data.(ChainView)
	if view.Address != "192.0.2.20:1081" {
		t.Fatalf("modify reported %q", view.Address)
	}
	if !slices.Contains(view.Users, "alice") {
		t.Fatalf("modify dropped the users that select it: %#v", view.Users)
	}

	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	chains := routing.ListChains(saved)
	if len(chains) != 1 || chains[0].Server != "192.0.2.20" || chains[0].ServerPort != 1081 {
		t.Fatalf("stored chain: %#v", chains)
	}
	rules := 0
	for _, rule := range saved.UserRoutes {
		if rule.Outbound == "relay-a" {
			rules++
		}
	}
	if rules != 1 {
		t.Fatalf("the rule stopped selecting the chain: %#v", saved.UserRoutes)
	}
	// The chain's DNS server still egresses through it, and kept the resolver
	// that was never named again.
	if _, known := routing.ChainResolverOf(saved, "relay-a"); !known {
		t.Fatal("the chain lost its DNS server")
	}

	// Credentials are replaced only when named, and a modification that names
	// nothing is an argument error rather than a silent no-op.
	if _, err := a.RoutingChainModify(ctx, "relay-a", ChainChange{}); err == nil {
		t.Fatal("a modification with nothing to change was accepted")
	}
	if _, err := a.RoutingChainModify(ctx, "ghost", ChainChange{Port: 1082}); err == nil {
		t.Fatal("a chain that does not exist was modified")
	}
	repeat, err := a.RoutingChainModify(ctx, "relay-a", ChainChange{Host: "192.0.2.20", Port: 1081})
	if err != nil || repeat.Changed {
		t.Fatalf("repeating a modification reported a change: %#v %v", repeat, err)
	}
}

// sync-dns upgrades a configuration written with the old fixed default and
// reports the strategy it wrote, not the one it replaced.
func TestSyncDNSReportsTheStrategyItResolved(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	s, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if routing.DirectStrategyResolved(s) {
		t.Skip("fixture already resolved")
	}
	result, err := a.RoutingSyncDNS(ctx)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	reported, _ := result.Data.(map[string]any)["strategy"].(string)
	if want := routeStrategy(saved); reported != want {
		t.Fatalf("reported %q, wrote %q", reported, want)
	}
}

// Whole-server chain mode: route.final names a chain, the DNS final follows it
// to the chain's resolver, the chain cannot be removed while it is final, and
// setting direct back restores both.
func TestRouteFinalThroughAChainMovesTheDNSFinal(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "res1", "192.0.2.10", 1080, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingFinal(ctx, "nosuch", true); err == nil {
		t.Fatal("an unknown outbound was accepted as final")
	}
	result, err := a.RoutingFinal(ctx, "res1", true)
	if err != nil || !result.Changed {
		t.Fatalf("set final: %#v %v", result, err)
	}
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.SingBox.Route.Final != "res1" || saved.SingBox.DNS.Final != "res1-dns" {
		t.Fatalf("route.final %q, dns.final %q", saved.SingBox.Route.Final, saved.SingBox.DNS.Final)
	}
	if saved.SingBox.Route.DefaultDomainResolver == "res1-dns" {
		t.Fatal("the default domain resolver went through the chain; it resolves the chain's own server")
	}
	if _, err := a.RoutingChainRemove(ctx, "res1"); err == nil {
		t.Fatal("the final chain was removed")
	}
	if result, err := a.RoutingFinal(ctx, "res1", true); err != nil || result.Changed {
		t.Fatalf("repeat set: %#v %v", result, err)
	}
	if _, err := a.RoutingFinal(ctx, "direct", true); err != nil {
		t.Fatal(err)
	}
	saved, _ = store.Load()
	if saved.SingBox.Route.Final != "direct" || saved.SingBox.DNS.Final == "res1-dns" {
		t.Fatalf("direct did not restore: route.final %q, dns.final %q", saved.SingBox.Route.Final, saved.SingBox.DNS.Final)
	}
	if _, err := a.RoutingChainRemove(ctx, "res1"); err != nil {
		t.Fatalf("a chain no longer final could not be removed: %v", err)
	}
}

// --rules takes the numbers route rule list prints, which are the preset
// menu's: several at once, letters in either case, never a preset name. The
// listing reads in menu order.
func TestRuleSelectorsAreThePresetMenuNumbers(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "res2", "192.0.2.20", 1080, "", "", ""); err != nil {
		t.Fatal(err)
	}
	// Added out of menu order on purpose.
	if _, err := a.RoutingSet(ctx, "alice", []string{"ai-intl", "xai", "twitter", "openai", "google", "anthropic"}, "direct"); err != nil {
		t.Fatal(err)
	}
	listed, err := a.RoutingList(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	selectors := []string{}
	for _, entry := range listed.Data.(map[string]any)["rules"].([]RouteEntry) {
		selectors = append(selectors, entry.Selector)
	}
	if got := strings.Join(selectors, ","); got != "1,2,3,6,j,a" {
		t.Fatalf("listing selectors = %s, want menu order 1,2,3,6,j,a", got)
	}
	result, err := a.RoutingRules(ctx, "alice", []string{"1", "2", "3", "6", "J", "a"}, "res2", false)
	if err != nil || result.Data.(map[string]any)["affected_rules"] != 6 {
		t.Fatalf("modify: %#v %v", result, err)
	}
	if _, err := a.RoutingRules(ctx, "alice", []string{"openai"}, "", true); err == nil {
		t.Fatal("a preset name was accepted where only numbers are")
	}
	if _, err := a.RoutingRules(ctx, "alice", []string{"1", "a"}, "", true); err != nil {
		t.Fatal(err)
	}
	listed, _ = a.RoutingList(ctx, "alice")
	for _, entry := range listed.Data.(map[string]any)["rules"].([]RouteEntry) {
		if entry.Selector == "1" || entry.Selector == "a" {
			t.Fatalf("removed rule %s is still listed", entry.Selector)
		}
		if entry.Outbound != "res2" {
			t.Fatalf("rule %s was not moved: %s", entry.Selector, entry.Outbound)
		}
	}
	// A preset alice has no rule for changes nothing, even beside ones she has.
	if _, err := a.RoutingRules(ctx, "alice", []string{"2", "b"}, "", true); err == nil {
		t.Fatal("a missing rule was accepted")
	}
	listed, _ = a.RoutingList(ctx, "alice")
	if len(listed.Data.(map[string]any)["rules"].([]RouteEntry)) != 4 {
		t.Fatal("a refused selection removed something")
	}
}

// add only creates: a preset the user already has is left on its outbound and
// reported as already added, while the new ones in the same call are added.
func TestRuleAddNeverRepointsAnExistingRule(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "res1", "192.0.2.10", 1080, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai", "google"}, "direct"); err != nil {
		t.Fatal(err)
	}
	result, err := a.RoutingSet(ctx, "alice", []string{"openai", "netflix"}, "res1")
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	if got := data["already_added"].([]string); len(got) != 1 || got[0] != "openai" {
		t.Fatalf("already_added = %v", got)
	}
	if got := data["presets"].([]string); len(got) != 1 || got[0] != "netflix" {
		t.Fatalf("presets = %v", got)
	}
	outbounds := map[string]string{}
	for _, entry := range data["rules"].([]RouteEntry) {
		outbounds[entry.Preset] = entry.Outbound
	}
	if outbounds["openai"] != "direct" || outbounds["netflix"] != "res1" || outbounds["google"] != "direct" {
		t.Fatalf("outbounds after add = %v; an existing rule was repointed or a new one missed", outbounds)
	}
	// Adding only what exists changes nothing.
	if again, err := a.RoutingSet(ctx, "alice", []string{"openai", "google"}, "res1"); err != nil || again.Changed {
		t.Fatalf("re-adding existing rules: %#v %v", again, err)
	}
}

// A chain still in use is refused with what uses it: every user's rules that
// select it, and whether it is the route final, so the caller can clear it in
// one pass instead of discovering each holder by trying again.
func TestChainRemoveNamesWhatStillUsesIt(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.RoutingChainAdd(ctx, "res1", "192.0.2.10", 1080, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai", "netflix"}, "res1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"google"}, "direct"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "bob", []string{"github"}, "res1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingFinal(ctx, "res1", true); err != nil {
		t.Fatal(err)
	}
	_, err := a.RoutingChainRemove(ctx, "res1")
	var refusal *Error
	if !errors.As(err, &refusal) || refusal.Code != "conflict" {
		t.Fatalf("referenced chain: %v", err)
	}
	fields := refusal.Data.(map[string]any)
	if fields["route_final"] != true || fields["tag"] != "res1" {
		t.Fatalf("refusal data = %#v", fields)
	}
	using := []string{}
	for _, entry := range fields["rules"].([]RouteEntry) {
		using = append(using, entry.User+":"+entry.Selector)
	}
	slices.Sort(using)
	if strings.Join(using, " ") != "alice:1 alice:b bob:9" {
		t.Fatalf("rules using the chain = %v", using)
	}
	chains, err := a.RoutingChains(ctx)
	if err != nil || !strings.Contains(fmt.Sprint(chains.Data), "res1") {
		t.Fatalf("the refused chain is gone: %#v %v", chains.Data, err)
	}

	// With the rules moved off it, the route final alone still holds it.
	if _, err := a.RoutingClear(ctx, "", true); err != nil {
		t.Fatal(err)
	}
	_, err = a.RoutingChainRemove(ctx, "res1")
	if !errors.As(err, &refusal) || !strings.Contains(refusal.Message, "route final") {
		t.Fatalf("route-final chain: %v", err)
	}
	if rules, _ := refusal.Data.(map[string]any)["rules"].([]RouteEntry); len(rules) != 0 {
		t.Fatalf("rules listed after they were removed: %v", rules)
	}
	if _, err := a.RoutingFinal(ctx, "direct", true); err != nil {
		t.Fatal(err)
	}
	if result, err := a.RoutingChainRemove(ctx, "res1"); err != nil || !result.Changed {
		t.Fatalf("unused chain: %#v %v", result, err)
	}
}

// route test reads remote rule sets out of sing-box's own cache, nested under
// the cache ID the way sing-box stores them, and matches with sing-box. A set
// the cache does not hold is reported, not treated as a miss.
func TestRouteTestMatchesAgainstSingBoxsCache(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	cachePath := filepath.Join(t.TempDir(), "cache.db")
	settings := `{"dns":{"strategy":"prefer_ipv6"},"outbounds":[{"type":"direct","tag":"direct"}],` +
		`"experimental":{"cache_file":{"enabled":true,"cache_id":"cache.db","path":"` + cachePath + `"}}}`
	if err := os.WriteFile(config.SingBoxConfig, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	// Answers as sing-box does, on stderr and exit 0 either way; the stub's
	// "rule set" is the list of names it holds. Builtins only: PATH is empty.
	stub := "#!/bin/sh\n[ \"$1\" = rule-set ] || exit 0\nIFS= read -r held < \"$5\"\n" +
		"case \" $held \" in *\" $6 \"*) echo 'match rules.[0]: domain/domain_suffix=<binary>' >&2 ;; esac\nexit 0\n"
	if err := os.WriteFile(config.SingBoxBin, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(cachePath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists(append([]byte{0}, "cache.db"...))
		if err != nil {
			return err
		}
		sets, err := root.CreateBucketIfNotExists([]byte("rule_set"))
		if err != nil {
			return err
		}
		for tag, held := range map[string]string{"geosite-openai": "chatgpt.com openai.com", "geosite-netflix": "netflix.com"} {
			// sing-box's SavedBinary: version, content, update time, etag, URL hash.
			saved := binary.AppendUvarint([]byte{2}, uint64(len(held)))
			saved = append(saved, held...)
			saved = binary.BigEndian.AppendUint64(saved, uint64(time.Now().Unix()))
			saved = append(saved, 0, 0)
			if err := sets.Put([]byte(tag), saved); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil || db.Close() != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingChainAdd(ctx, "res1", "192.0.2.10", 1080, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"google"}, "direct"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RoutingSet(ctx, "alice", []string{"openai", "netflix"}, "res1"); err != nil {
		t.Fatal(err)
	}

	result, err := a.RoutingTest(ctx, "alice", "ChatGPT.com.")
	if err != nil {
		t.Fatal(err)
	}
	fields := result.Data.(map[string]any)
	evaluation := fields["evaluation"].(routing.Evaluation)
	decision := evaluation.Decision
	if evaluation.Target != "chatgpt.com" || decision.MatchBy != "rule_set" || decision.Value != "geosite-openai" || decision.Outbound != "res1" {
		t.Fatalf("chatgpt.com: %#v", evaluation)
	}
	if rule, _ := fields["rule"].(RouteEntry); rule.Selector != "1" || fields["address"] != "192.0.2.10:1080" {
		t.Fatalf("deciding rule %#v at %v", fields["rule"], fields["address"])
	}

	result, err = a.RoutingTest(ctx, "alice", "unlisted.example")
	if err != nil {
		t.Fatal(err)
	}
	evaluation = result.Data.(map[string]any)["evaluation"].(routing.Evaluation)
	if evaluation.Decision.MatchBy != "final" || evaluation.Decision.Outbound != "direct" ||
		!slices.Contains(evaluation.Unchecked, "geosite-google") || slices.Contains(evaluation.Unchecked, "geosite-openai") {
		t.Fatalf("unlisted.example: %#v", evaluation)
	}
}

// A custom-port request reports what it did to each transport it named, so
// "both" answers twice, and a repeat or a removal of something absent says so
// instead of passing for a change.
func TestFirewallPortReportsEachTransport(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	results := func(result Result) string {
		parts := []string{}
		for _, change := range result.Data.(map[string]any)["changes"].([]FirewallPortChange) {
			parts = append(parts, fmt.Sprintf("%d/%s %s", change.Port, change.Transport, change.Result))
		}
		return strings.Join(parts, "; ")
	}
	steps := []struct {
		transport string
		remove    bool
		want      string
		changed   bool
	}{
		{"tcp", false, "8443/tcp added", true},
		{"both", false, "8443/tcp already added; 8443/udp added", true},
		{"tcp", false, "8443/tcp already added", false},
		{"udp", true, "8443/udp removed", true},
		{"both", true, "8443/tcp removed; 8443/udp not found", true},
		{"tcp", true, "8443/tcp not found", false},
	}
	for _, step := range steps {
		result, err := a.NetworkFirewallPort(ctx, 8443, step.transport, step.remove)
		if err != nil {
			t.Fatal(err)
		}
		if got := results(result); got != step.want || result.Changed != step.changed {
			t.Fatalf("%s remove=%v: %q changed=%v, want %q changed=%v", step.transport, step.remove, got, result.Changed, step.want, step.changed)
		}
		// No custom port left is an empty list, not null.
		if ports, _ := result.Data.(map[string]any)["ports"].([]store.FirewallPort); ports == nil {
			t.Fatalf("%s remove=%v: ports is nil", step.transport, step.remove)
		}
	}
}

// Caddy's site cannot move onto a node's port, onto the challenge port or
// onto nothing parseable, and naming the port it already has changes
// nothing.
func TestCertificatePortRefusesPortsThatAreNotCaddys(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	if _, err := a.CertificatePort(ctx, ""); err == nil {
		t.Fatal("reported a port with no Caddyfile")
	}
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{"inbounds":[{"type":"anytls","tag":"anytls-443","listen_port":443}],"outbounds":[{"type":"direct","tag":"direct"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.CaddyFile, []byte("{\n}\n\na.example.org:18443 {\n    file_server\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.DomainFile, []byte("a.example.org\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := a.CertificatePort(ctx, "")
	if data, _ := result.Data.(map[string]any); err != nil || data["port"] != 18443 {
		t.Fatalf("show: %#v %v", result, err)
	}
	for value, refusal := range map[string]string{"443": "belongs to anytls-443", "80": "certificate issuance", "0": "1 to 65535", "x": "1 to 65535"} {
		_, err := a.CertificatePort(ctx, value)
		var detail *Error
		if !errors.As(err, &detail) || detail.Code != "invalid_argument" || !strings.Contains(detail.Message, refusal) {
			t.Fatalf("port %s: %v", value, err)
		}
	}
	if result, err := a.CertificatePort(ctx, "18443"); err != nil || result.Changed {
		t.Fatalf("same port: %#v %v", result, err)
	}
	// Nor can a node take caddy's port, even while caddy is stopped.
	_, err = a.ProtocolInstall(ctx, ProtocolOptions{Type: protocol.VLESS, User: "alice", Port: "18443", Domain: "a.example.org"})
	if err == nil || !strings.Contains(err.Error(), "belongs to caddy") {
		t.Fatalf("a node was offered caddy's port: %v", err)
	}
}
