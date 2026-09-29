package routing

import (
	"encoding/json"
	"strings"
	"testing"

	"go-proxy/internal/store"
)

func TestUserRouteLabelUsesPresetLabel(t *testing.T) {
	preset, ok := FindPreset("openai")
	if !ok {
		t.Fatal("FindPreset(openai) = false")
	}
	rule := PresetToRule(preset, "alice", store.DirectTag)
	if got := UserRouteLabel(rule); got != preset.Label {
		t.Fatalf("UserRouteLabel() = %q, want %q", got, preset.Label)
	}
}

func TestCompiledUserRouteRulesKeepGeositeAndGeoIPGrouped(t *testing.T) {
	s := setupRoutingStore(t)
	preset, ok := FindPreset("google")
	if !ok {
		t.Fatal("FindPreset(google) = false")
	}
	s.UserRoutes = []store.UserRouteRule{
		PresetToRule(preset, "alice", "proxy-a"),
	}

	rules := CompiledUserRouteRules(s)
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(rules))
	}
	if got := rules[0].RuleSet; len(got) != 1 || got[0] != "geosite-google" {
		t.Fatalf("rules[0].RuleSet = %#v", got)
	}
	if got := rules[1].RuleSet; len(got) != 1 || got[0] != "geoip-google" {
		t.Fatalf("rules[1].RuleSet = %#v", got)
	}
	if len(rules[0].DomainSuffix) != 0 || len(rules[1].DomainSuffix) != 0 {
		t.Fatalf("compiled preset rules should not keep fallback domain_suffix when rulesets exist")
	}
}

func TestDNSRulesMatchCompiledRouteRules(t *testing.T) {
	s := setupRoutingStore(t)
	preset, ok := FindPreset("google")
	if !ok {
		t.Fatal("FindPreset(google) = false")
	}
	s.UserRoutes = []store.UserRouteRule{
		PresetToRule(preset, "alice", "proxy-a"),
	}

	dnsRules := mergeDNSRulesByServer(dnsRulesFromRouteRules(CompiledUserRouteRules(s), map[string]string{"proxy-a": "dns-proxy"}, ChainDNSStrategies(s), "ipv4_only"))
	if len(dnsRules) != 2 {
		t.Fatalf("len(dnsRules) = %d, want 2", len(dnsRules))
	}
	if got := dnsRules[0].rule.RuleSet; len(got) != 1 || got[0] != "geosite-google" {
		t.Fatalf("dnsRules[0].RuleSet = %#v", got)
	}
	if got := dnsRules[1].rule.RuleSet; len(got) != 1 || got[0] != "geoip-google" {
		t.Fatalf("dnsRules[1].RuleSet = %#v", got)
	}
}

// A rule that names only part of its preset is not that preset: matching a
// subset would let one preset claim another's rule, and nothing writes a
// partial rule now that PresetToRule stores the whole set.
func TestPartialRuleSetIsNotItsPreset(t *testing.T) {
	preset, ok := FindPreset("ai-intl")
	if !ok {
		t.Fatal("FindPreset(ai-intl) = false")
	}
	full := PresetToRule(preset, "alice", "proxy-a")
	if UserRouteLabel(full) != preset.Label {
		t.Fatalf("a rule built from the preset did not match it: %q", UserRouteLabel(full))
	}
	partial := full
	partial.RuleSet = []string{"geosite-category-ai-!cn"}
	if got := UserRouteLabel(partial); got == preset.Label {
		t.Fatalf("a partial rule claimed the preset label %q", got)
	}
}

// matchSets is a RuleSetMatcher over a fixed table: which sets hold which
// names. It records what it was asked, so a test can see a set was never
// consulted.
func matchSets(holds map[string][]string, target string, asked *[]string) RuleSetMatcher {
	return func(tags []string) (string, []string, error) {
		for _, tag := range tags {
			*asked = append(*asked, tag)
			for _, name := range holds[tag] {
				if name == target {
					return tag, nil, nil
				}
			}
		}
		return "", nil, nil
	}
}

// A decision has to report both halves of where the traffic goes: the
// outbound it takes and the resolver its name goes through. Reporting only the
// outbound hides a lookup leaving by a different address than its traffic.
func TestEvaluateReportsTheResolverBesideTheOutbound(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "res-a", "198.51.100.10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	for name, outbound := range map[string]string{"openai": "res-a", "netflix": store.DirectTag} {
		preset, ok := FindPreset(name)
		if !ok {
			t.Fatalf("FindPreset(%s) = false", name)
		}
		if err := SetRule(s, "alice", PresetToRule(preset, "alice", outbound)); err != nil {
			t.Fatal(err)
		}
	}
	Sync(s)
	holds := map[string][]string{"geosite-openai": {"chat.openai.com"}, "geosite-netflix": {"www.netflix.com"}}

	var asked []string
	chained, err := Evaluate(s, "alice", "chat.openai.com", matchSets(holds, "chat.openai.com", &asked))
	if err != nil {
		t.Fatal(err)
	}
	decision := chained.Decision
	if decision.MatchBy != "rule_set" || decision.Value != "geosite-openai" || decision.Outbound != "res-a" {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.DNSServer != ChainDNSTag("res-a") || decision.DNSVia != "res-a" {
		t.Fatalf("resolver %q via %q for traffic through res-a", decision.DNSServer, decision.DNSVia)
	}

	// A direct rule resolves directly: no detour, which is the synchronised
	// case for direct rather than a mismatch.
	plain, err := Evaluate(s, "alice", "www.netflix.com", matchSets(holds, "www.netflix.com", &asked))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Decision.Outbound != "direct" || plain.Decision.DNSVia != "" || plain.Decision.DNSServer == "" {
		t.Fatalf("direct decision = %#v", plain.Decision)
	}
}

// Evaluate follows sing-box: compiled rules in order, the first that takes
// the connection wins, rules for other users are passed over, IP rules never
// decide a name, and what no rule takes goes to the route final.
func TestEvaluateFollowsTheCompiledRuleOrder(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "res-a", "198.51.100.10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	s.SingBox.Route.Rules = []store.RouteRule{
		{Action: "sniff"},
		{Action: "hijack-dns", Protocol: "dns"},
		{Action: "route", Outbound: store.DirectTag, IPIsPrivate: true},
		{Action: "route", Outbound: "res-a", AuthUser: []string{"bob"}, RuleSet: []string{"geosite-google"}},
		{Action: "route", Outbound: store.DirectTag, AuthUser: []string{"alice"}, RuleSet: []string{"geosite-google"}},
		{Action: "route", Outbound: "res-a", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-youtube"}},
		{Action: "route", Outbound: "res-a", AuthUser: []string{"alice"}, RuleSet: []string{"geoip-google"}},
		{Action: "route", Outbound: "res-a", AuthUser: []string{"alice"}, DomainSuffix: []string{".example.org"}},
	}
	// youtube.com is in both sets; the earlier rule takes it.
	holds := map[string][]string{
		"geosite-google": {"youtube.com"}, "geosite-youtube": {"youtube.com"}, "geoip-google": {"8.8.8.8"},
	}
	cases := []struct {
		user, target, by, value, outbound string
	}{
		{"alice", "youtube.com", "rule_set", "geosite-google", "direct"},
		{"bob", "youtube.com", "rule_set", "geosite-google", "res-a"},
		{"alice", "8.8.8.8", "rule_set", "geoip-google", "res-a"},
		{"alice", "10.0.0.7", "ip_is_private", "", "direct"},
		{"alice", "www.example.org", "domain_suffix", ".example.org", "res-a"},
		// ".example.org" takes names under it, not the name itself.
		{"alice", "example.org", "final", "", "direct"},
		{"carol", "youtube.com", "final", "", "direct"},
	}
	for _, c := range cases {
		var asked []string
		result, err := Evaluate(s, c.user, c.target, matchSets(holds, c.target, &asked))
		if err != nil {
			t.Fatal(err)
		}
		got := result.Decision
		if got.MatchBy != c.by || got.Value != c.value || got.Outbound != c.outbound {
			t.Fatalf("%s %s: decision %#v, want %s %s -> %s", c.user, c.target, got, c.by, c.value, c.outbound)
		}
		for _, tag := range asked {
			if strings.HasPrefix(tag, "geoip-") && result.Kind == "domain" {
				t.Fatalf("%s: an IP set was consulted for a name: %v", c.target, asked)
			}
		}
	}

	// A chain set as the route final takes what no rule does.
	s.SingBox.Route.Final = "res-a"
	var asked []string
	result, err := Evaluate(s, "carol", "anything.example", matchSets(nil, "", &asked))
	if err != nil || result.Decision.Rule != -1 || result.Decision.Outbound != "res-a" {
		t.Fatalf("final chain: %#v %v", result.Decision, err)
	}
}

// A preset whose rule set is available compiles without its fallback
// domains, so a name only the fallback list carries is not taken by it.
func TestEvaluateIgnoresFallbackDomainsSingBoxNeverSees(t *testing.T) {
	s := setupRoutingStore(t)
	preset, ok := FindPreset("openai")
	if !ok {
		t.Fatal("FindPreset(openai) = false")
	}
	if err := SetRule(s, "alice", PresetToRule(preset, "alice", store.DirectTag)); err != nil {
		t.Fatal(err)
	}
	Sync(s)
	var asked []string
	result, err := Evaluate(s, "alice", preset.FallbackDomains[0], matchSets(nil, "", &asked))
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.MatchBy != "final" {
		t.Fatalf("a fallback domain decided: %#v", result.Decision)
	}
}

// Each rule takes its sing-box 1.14 form: no strategy on any rule; a single-
// family chain answers the other family's queries for its domains with an
// empty answer and evaluates only its own family's queries for its address
// sets; address sets match the evaluated answer, which respond returns.
func TestDNSRulesTakeTheSingBox114Form(t *testing.T) {
	users := []string{"alice"}
	routes := []dnsRoute{
		{rule: store.DNSRule{Action: "route", Server: "res-dns", AuthUser: users, RuleSet: []string{"geosite-openai"}}, strategy: "ipv4_only"},
		{rule: store.DNSRule{Action: "route", Server: "res-dns", AuthUser: users, RuleSet: []string{"geoip-ai"}}, strategy: "ipv4_only"},
		{rule: store.DNSRule{Action: "route", Server: "public4", AuthUser: users, RuleSet: []string{"geosite-netflix"}}, strategy: "prefer_ipv4"},
	}
	got, _ := json.Marshal(dnsRulesFor(routes))
	want := `[` +
		`{"action":"predefined","auth_user":["alice"],"query_type":["AAAA"],"rule_set":["geosite-openai"]},` +
		`{"action":"route","server":"res-dns","auth_user":["alice"],"rule_set":["geosite-openai"]},` +
		`{"action":"evaluate","server":"res-dns","tag":"res-dns-geoip-1","auth_user":["alice"],"query_type":["A"]},` +
		`{"action":"respond","auth_user":["alice"],"rule_set":["geoip-ai"],"match_response":"res-dns-geoip-1"},` +
		`{"action":"route","server":"public4","auth_user":["alice"],"rule_set":["geosite-netflix"]}` +
		`]`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// gstatic.com leaves directly even for a user whose Google rule goes through
// a chain; the rest of Google still takes the chain.
func TestGstaticLeavesDirectlyAheadOfAChainedGoogleRule(t *testing.T) {
	s := setupRoutingStore(t)
	if err := AddChain(s, "res-a", "198.51.100.10", 1080, "", ""); err != nil {
		t.Fatal(err)
	}
	preset, ok := FindPreset("google")
	if !ok {
		t.Fatal("FindPreset(google) = false")
	}
	if err := SetRule(s, "alice", PresetToRule(preset, "alice", "res-a")); err != nil {
		t.Fatal(err)
	}
	Sync(s)
	s.SingBox.Normalize()
	holds := map[string][]string{"geosite-google": {"www.gstatic.com", "www.google.com"}}
	for target, want := range map[string]string{"www.gstatic.com": store.DirectTag, "connectivitycheck.gstatic.com": store.DirectTag, "www.google.com": "res-a"} {
		var asked []string
		result, err := Evaluate(s, "alice", target, matchSets(holds, target, &asked))
		if err != nil {
			t.Fatal(err)
		}
		if result.Decision.Outbound != want {
			t.Fatalf("%s leaves by %q (%#v), want %q", target, result.Decision.Outbound, result.Decision, want)
		}
	}
}
