package routing

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	"go-proxy/internal/store"
)

func TestClearUserRemovesMembershipFromSharedRule(t *testing.T) {
	s := setupRoutingStore(t)
	s.UserRoutes = []store.UserRouteRule{{AuthUser: []string{"alice", "bob"}, Outbound: "direct", DomainSuffix: []string{"example.com"}}}
	if got := ClearUser(s, "alice"); got != 1 {
		t.Fatalf("removed %d memberships, want 1", got)
	}
	if len(s.UserRoutes) != 1 || len(s.UserRoutes[0].AuthUser) != 1 || s.UserRoutes[0].AuthUser[0] != "bob" {
		t.Fatalf("other user rule changed: %#v", s.UserRoutes)
	}
	if !s.IsDirtyFile(store.FileUserRoutes) {
		t.Fatal("membership removal was not marked for persistence")
	}
}

func TestRepeatedRouteSetPreservesSharedRule(t *testing.T) {
	s := setupRoutingStore(t)
	preset, _ := FindPreset("openai")
	rule := PresetToRule(preset, "alice", "direct")
	rule.AuthUser = append(rule.AuthUser, "bob")
	s.UserRoutes = []store.UserRouteRule{rule}
	if err := SetRule(s, "alice", PresetToRule(preset, "alice", "direct")); err != nil {
		t.Fatal(err)
	}
	if s.IsDirty() || len(s.UserRoutes) != 1 || len(s.UserRoutes[0].AuthUser) != 2 {
		t.Fatal("repeated route request must preserve shared membership")
	}
}

// A chain resolves for the families its own endpoint has, so the global
// strategy governs direct rules and the chain governs its own. The global one
// must survive either way.
func TestNamedChainSyncPreservesStrategyAndRemovesOwnedDNS(t *testing.T) {
	for _, strategy := range []string{"", "ipv4_only", "ipv6_only", "prefer_ipv4", "prefer_ipv6"} {
		t.Run(strategy, func(t *testing.T) {
			s := setupRoutingStore(t)
			s.SingBox.DNS.Strategy = strategy
			if err := AddChain(s, "relay-a", "192.0.2.10", 1080, "user", "test-password"); err != nil {
				t.Fatal(err)
			}
			preset, _ := FindPreset("openai")
			if err := SetRule(s, "alice", PresetToRule(preset, "alice", "relay-a")); err != nil {
				t.Fatal(err)
			}
			Sync(s)
			if s.SingBox.DNS.Strategy != strategy {
				t.Fatal("global strategy changed")
			}
			found := false
			for _, rule := range s.SingBox.DNS.Rules {
				if len(rule.AuthUser) > 0 {
					found = true
					// The endpoint is IPv4, so its lookups ask for IPv4 alone
					// whatever the host's own direct strategy is.
					if rule.Server != ChainDNSTag("relay-a") || rule.Strategy != "ipv4_only" {
						t.Fatalf("incorrect DNS route: %#v", rule)
					}
				}
			}
			if !found {
				t.Fatal("no user DNS route")
			}
			if got := ListChains(s); len(got) != 1 || got[0].Tag != "relay-a" {
				t.Fatalf("named chain missing: %#v", got)
			}
			ClearUser(s, "alice")
			if err := RemoveChain(s, "relay-a"); err != nil {
				t.Fatal(err)
			}
			Sync(s)
			for _, raw := range s.SingBox.DNS.Servers {
				var server struct {
					Tag string `json:"tag"`
				}
				if err := json.Unmarshal(raw, &server); err != nil {
					t.Fatal(err)
				}
				if server.Tag == ChainDNSTag("relay-a") {
					t.Fatal("removed chain left DNS server behind")
				}
			}
		})
	}
}

// A rule set sing-box has not cached cannot be looked into. The rule that
// needs it is neither matched nor skipped silently: the set is named, and the
// decision reached without it says it holds only if that set does not match.
func TestEvaluateNamesTheRuleSetsItCouldNotCheck(t *testing.T) {
	s := setupRoutingStore(t)
	s.UserRoutes = []store.UserRouteRule{{AuthUser: []string{"alice"}, Outbound: store.DirectTag, RuleSet: []string{"geosite-openai"}}}
	Sync(s)
	uncached := func(tags []string) (string, []string, error) { return "", tags, nil }
	result, err := Evaluate(s, "alice", "unrelated.example", uncached)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.MatchBy != "final" || len(result.Unchecked) != 1 || result.Unchecked[0] != "geosite-openai" {
		t.Fatalf("rule-set availability must remain explicit: %#v", result)
	}
}

func TestNamedChainRejectsPortAndTagCollision(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "relay-a", "example.com", 65536, "", ""); err == nil {
		t.Fatal("out-of-range port accepted")
	}
	if err := AddChain(s, "relay-a", "example.com", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := AddChain(s, "relay-a", "other.example", 1080, "", ""); err == nil {
		t.Fatal("duplicate chain accepted")
	}
}

// A chain's lookups ask for its endpoint's family alone: an IPv4 endpoint
// cannot carry a AAAA answer, so a preference that still allowed one would
// resolve to an address the chain cannot reach.
func TestChainDNSStrategyFollowsTheEndpointFamily(t *testing.T) {
	for _, testCase := range []struct{ server, want string }{
		{"192.0.2.10", "ipv4_only"},
		{"2001:db8::1", "ipv6_only"},
		{"proxy.example.net", ""}, // a hostname is decided where it can be resolved
	} {
		if got := ChainStrategy(testCase.server); got != testCase.want {
			t.Fatalf("ChainStrategy(%q) = %q, want %q", testCase.server, got, testCase.want)
		}
	}

	// The strategy recorded on a chain's DNS server is what its rules use, and
	// a direct rule keeps the configured one.
	s := setupRoutingStore(t)
	s.SingBox.DNS.Strategy = "ipv6_only"
	if err := AddChain(s, "v4-chain", "192.0.2.10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	preset, _ := FindPreset("openai")
	if err := SetRule(s, "alice", PresetToRule(preset, "alice", "v4-chain")); err != nil {
		t.Fatal(err)
	}
	netflix, _ := FindPreset("netflix")
	if err := SetRule(s, "alice", PresetToRule(netflix, "alice", "direct")); err != nil {
		t.Fatal(err)
	}
	Sync(s)
	seen := map[string]string{}
	for _, rule := range s.SingBox.DNS.Rules {
		if len(rule.AuthUser) > 0 {
			seen[rule.Server] = rule.Strategy
		}
	}
	if got := seen[ChainDNSTag("v4-chain")]; got != "ipv4_only" {
		t.Fatalf("chain rule strategy = %q, want ipv4_only", got)
	}
	if got := seen["public4"]; got != "ipv6_only" {
		t.Fatalf("direct rule strategy = %q, want the configured ipv6_only", got)
	}

	// A hostname endpoint is decided by whatever resolved it, which records
	// its answer, and the rules then follow it. An address decides itself.
	if err := AddChain(s, "by-name", "proxy.example.net", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	SetChainDNS(s, "by-name", DefaultChainResolver(), "ipv6_only")
	if got := ChainDNSStrategies(s)[ChainDNSTag("by-name")]; got != "ipv6_only" {
		t.Fatalf("recorded strategy = %q, want ipv6_only", got)
	}
	SetChainDNS(s, "v4-chain", DefaultChainResolver(), "ipv6_only")
	if got := ChainDNSStrategies(s)[ChainDNSTag("v4-chain")]; got != "ipv4_only" {
		t.Fatalf("an address endpoint took a recorded strategy: %q", got)
	}
}

// A chain's DNS server carries a detour, and sing-box will not resolve the
// resolver's own hostname through one. A name is therefore pinned to an
// address when the chain is added and kept only as the certificate name.
func TestParseChainResolverPinsNamesAndRejectsWhatSingBoxRefuses(t *testing.T) {
	lookup := func(name string) (net.IP, error) {
		if name == "dns.quad9.net" {
			return net.ParseIP("9.9.9.9"), nil
		}
		return nil, errors.New("no such host")
	}
	for _, testCase := range []struct {
		in   string
		want ChainResolver
	}{
		{"", DefaultChainResolver()},
		{"10.0.0.53", ChainResolver{Type: "udp", Server: "10.0.0.53", ServerPort: 53}},
		{"10.0.0.53:5353", ChainResolver{Type: "udp", Server: "10.0.0.53", ServerPort: 5353}},
		{"tcp://10.0.0.53", ChainResolver{Type: "tcp", Server: "10.0.0.53", ServerPort: 53}},
		{"tls://1.1.1.1", ChainResolver{Type: "tls", Server: "1.1.1.1", ServerPort: 853}},
		{"https://9.9.9.9/dns-query", ChainResolver{Type: "https", Server: "9.9.9.9", ServerPort: 443, Path: "/dns-query"}},
		// The name survives as TLSName; the address is what gets dialled.
		{"https://dns.quad9.net/dns-query", ChainResolver{Type: "https", Server: "9.9.9.9", ServerPort: 443, Path: "/dns-query", TLSName: "dns.quad9.net"}},
		{"https://9.9.9.9", ChainResolver{Type: "https", Server: "9.9.9.9", ServerPort: 443, Path: "/dns-query"}},
	} {
		got, err := ParseChainResolver(testCase.in, lookup)
		if err != nil {
			t.Fatalf("ParseChainResolver(%q): %v", testCase.in, err)
		}
		if got != testCase.want {
			t.Fatalf("ParseChainResolver(%q) = %#v, want %#v", testCase.in, got, testCase.want)
		}
	}
	for _, bad := range []string{"sctp://1.1.1.1", "10.0.0.53:0", "10.0.0.53:99999", "udp://1.1.1.1/dns-query", "://x", "nowhere.invalid"} {
		if got, err := ParseChainResolver(bad, lookup); err == nil {
			t.Fatalf("ParseChainResolver(%q) accepted %#v", bad, got)
		}
	}
	// Without a way to look up, a name is refused rather than dropped.
	if _, err := ParseChainResolver("https://dns.quad9.net/dns-query", nil); err == nil {
		t.Fatal("a hostname resolver was accepted with no way to resolve it")
	}
}

// TLS is declared only for the transports that use it, and only with a name:
// a certificate cannot be checked against a bare address.
func TestChainDNSServerDeclaresTLSOnlyWhenItApplies(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "c", "192.0.2.10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		resolver ChainResolver
		wantTLS  bool
	}{
		{DefaultChainResolver(), true},
		{ChainResolver{Type: "udp", Server: "10.0.0.53", ServerPort: 53}, false},
		{ChainResolver{Type: "https", Server: "9.9.9.9", ServerPort: 443, Path: "/dns-query"}, false},
		{ChainResolver{Type: "tls", Server: "1.1.1.1", ServerPort: 853, TLSName: "one.one.one.one"}, true},
	} {
		SetChainDNS(s, "c", testCase.resolver, "ipv4_only")
		var found map[string]any
		for _, raw := range s.SingBox.DNS.Servers {
			var srv map[string]any
			if json.Unmarshal(raw, &srv) == nil && srv["tag"] == ChainDNSTag("c") {
				found = srv
			}
		}
		if found == nil {
			t.Fatalf("no chain DNS server for %#v", testCase.resolver)
		}
		if _, has := found["tls"]; has != testCase.wantTLS {
			t.Fatalf("%#v produced tls=%v, want %v", testCase.resolver, has, testCase.wantTLS)
		}
		if found["detour"] != "c" {
			t.Fatalf("resolver does not go through its chain: %#v", found)
		}
		// Never a hostname: sing-box cannot resolve one behind a detour.
		if net.ParseIP(found["server"].(string)) == nil {
			t.Fatalf("resolver server is not an address: %#v", found)
		}
	}
	// One server per chain however many times it is written.
	count := 0
	for _, raw := range s.SingBox.DNS.Servers {
		var srv struct {
			Tag string `json:"tag"`
		}
		if json.Unmarshal(raw, &srv) == nil && srv.Tag == ChainDNSTag("c") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d DNS servers for one chain", count)
	}
}

// An endpoint that is an address decides its own families, so a chain added
// before that rule changed converges on the next sync. A resolver the operator
// chose is not disturbed by that rewrite.
func TestSyncReDerivesStrategyForAddressEndpointsOnly(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "v4", "192.0.2.10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	chosen := ChainResolver{Type: "udp", Server: "10.0.0.53", ServerPort: 53}
	SetChainDNS(s, "v4", chosen, "prefer_ipv4") // as an older build recorded it
	Sync(s)
	if got := ChainDNSStrategies(s)[ChainDNSTag("v4")]; got != "ipv4_only" {
		t.Fatalf("strategy = %q, want the re-derived ipv4_only", got)
	}
	if got := ChainResolvers(s)["v4"]; got != "udp 10.0.0.53:53" {
		t.Fatalf("the chosen resolver was lost: %q", got)
	}

	// A hostname endpoint has nothing to re-derive from here, so whatever
	// resolved it stands.
	if err := AddChain(s, "byname", "proxy.example.net", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	SetChainDNS(s, "byname", DefaultChainResolver(), "prefer_ipv4")
	Sync(s)
	if got := ChainDNSStrategies(s)[ChainDNSTag("byname")]; got != "prefer_ipv4" {
		t.Fatalf("a hostname chain lost its recorded strategy: %q", got)
	}
}

// A chain reached only over IPv6 gets the default resolver at Google's IPv6
// address; one reached over IPv4 keeps 8.8.8.8.
func TestDefaultChainResolverFollowsTheEndpointFamily(t *testing.T) {
	s := setupRoutingStore(t)
	for tag, host := range map[string]string{"v4": "192.0.2.10", "v6": "2001:db8::10"} {
		if err := AddChain(s, tag, host, 1080, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	Sync(s)
	resolvers := ChainResolvers(s)
	if got := resolvers["v4"]; got != "https dns.google 8.8.8.8:443" {
		t.Fatalf("v4 chain resolver = %q", got)
	}
	if got := resolvers["v6"]; got != "https dns.google [2001:4860:4860::8888]:443" {
		t.Fatalf("v6 chain resolver = %q", got)
	}
}

// A chain DNS server written by an earlier gproxy converges on the next sync:
// renamed to "<tag>-dns", its domain_strategy moved into the chain record, and
// the default resolver moved to the family the chain reaches. The rest of the
// entry keeps its keys in order.
func TestSyncMigratesALegacyChainDNSServer(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "v6", "2001:db8::10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := AddChain(s, "byname", "proxy.example.net", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	s.SingBox.DNS.Servers = append(s.SingBox.DNS.Servers,
		json.RawMessage(`{"tag":"gproxy-chain-v6","type":"https","server":"8.8.8.8","server_port":443,"path":"/dns-query","tls":{"enabled":true,"server_name":"dns.google"},"detour":"v6","domain_strategy":"ipv6_only"}`),
		json.RawMessage(`{"tag":"gproxy-chain-byname","type":"https","server":"8.8.8.8","server_port":443,"path":"/dns-query","tls":{"enabled":true,"server_name":"dns.google"},"detour":"byname","domain_strategy":"prefer_ipv4"}`))
	Sync(s)
	byTag := map[string]string{}
	for _, raw := range s.SingBox.DNS.Servers {
		var srv struct {
			Tag string `json:"tag"`
		}
		_ = json.Unmarshal(raw, &srv)
		byTag[srv.Tag] = string(raw)
	}
	if got := byTag["v6-dns"]; got != `{"tag":"v6-dns","type":"https","server":"2001:4860:4860::8888","server_port":443,"path":"/dns-query","tls":{"enabled":true,"server_name":"dns.google"},"detour":"v6"}` {
		t.Fatalf("v6 server = %s", got)
	}
	if strings.Contains(byTag["byname-dns"], "domain_strategy") || byTag["byname-dns"] == "" {
		t.Fatalf("byname server = %q", byTag["byname-dns"])
	}
	for tag := range byTag {
		if strings.HasPrefix(tag, "gproxy-chain-") {
			t.Fatalf("legacy server survived: %s", tag)
		}
	}
	if got := ChainStrategies(s)["byname"]; got != "prefer_ipv4" {
		t.Fatalf("hostname chain strategy = %q, want the migrated prefer_ipv4", got)
	}
	for _, rule := range s.SingBox.DNS.Rules {
		if rule.Server == "gproxy-chain-v6" {
			t.Fatalf("a rule still names the legacy server: %+v", rule)
		}
	}
}
