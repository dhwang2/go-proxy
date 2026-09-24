package routing

import (
	"slices"
	"strings"
	"testing"

	"go-proxy/internal/store"
)

func TestSyncRouteRulesPreservesMatchersAcrossRuleSetSplits(t *testing.T) {
	s := setupRoutingStore(t)
	s.SingBox.Route = &store.RouteConfig{}
	s.UserRoutes = []store.UserRouteRule{
		{
			Action:        "route",
			Outbound:      "proxy-a",
			AuthUser:      []string{"alice"},
			RuleSet:       []string{"geosite-openai", "geoip-openai", "custom-rs"},
			Domain:        []string{"api.openai.com"},
			DomainSuffix:  []string{"openai.com"},
			DomainKeyword: []string{"chatgpt"},
			DomainRegex:   []string{".*\\.claude\\.ai$"},
			IPCIDR:        []string{"1.1.1.0/24"},
		},
	}

	syncRouteRules(s, CompiledUserRouteRules(s))

	if len(s.SingBox.Route.Rules) != 3 {
		t.Fatalf("len(Route.Rules) = %d, want 3", len(s.SingBox.Route.Rules))
	}

	for i, rule := range s.SingBox.Route.Rules {
		if got := rule.Domain; len(got) != 1 || got[0] != "api.openai.com" {
			t.Fatalf("rule %d Domain = %#v", i, got)
		}
		if got := rule.DomainSuffix; len(got) != 1 || got[0] != "openai.com" {
			t.Fatalf("rule %d DomainSuffix = %#v", i, got)
		}
		if got := rule.DomainKeyword; len(got) != 1 || got[0] != "chatgpt" {
			t.Fatalf("rule %d DomainKeyword = %#v", i, got)
		}
		if got := rule.DomainRegex; len(got) != 1 || got[0] != ".*\\.claude\\.ai$" {
			t.Fatalf("rule %d DomainRegex = %#v", i, got)
		}
		if got := rule.IPCIDR; len(got) != 1 || got[0] != "1.1.1.0/24" {
			t.Fatalf("rule %d IPCIDR = %#v", i, got)
		}
	}
}

// The direct strategy follows the host: both families prefer IPv4, one family
// asks for that family alone, and the IPv6-only host resolves through the
// IPv6 server. All four places sing-box reads the choice agree.
func TestResolveDirectStrategyFollowsTheHost(t *testing.T) {
	for _, tc := range []struct{ detected, resolver string }{
		{"prefer_ipv4", "public4"},
		{"ipv4_only", "public4"},
		{"ipv6_only", "public6"},
	} {
		t.Run(tc.detected, func(t *testing.T) {
			s := setupRoutingStore(t)
			s.SingBox.DNS.Strategy = "ipv4_only" // the old fixed default
			if !ResolveDirectStrategy(s, tc.detected) {
				t.Fatal("an unchosen configuration was not resolved")
			}
			if s.SingBox.DNS.Strategy != tc.detected || s.SingBox.DNS.Final != tc.resolver || s.SingBox.Route.DefaultDomainResolver != tc.resolver {
				t.Fatalf("dns %q/%q, route resolver %q", s.SingBox.DNS.Strategy, s.SingBox.DNS.Final, s.SingBox.Route.DefaultDomainResolver)
			}
			if server, strategy, ok := s.SingBox.DirectResolver(); !ok || server != tc.resolver || strategy != tc.detected {
				t.Fatalf("direct outbound resolver = %q %q %v", server, strategy, ok)
			}
			if ResolveDirectStrategy(s, "ipv4_only") {
				t.Fatal("a resolved configuration was resolved again")
			}
		})
	}
}

// A strategy other than the old default was someone's choice. It is kept and
// the resolvers are brought into line with it; an unreadable host leaves the
// old default for the next change to resolve.
func TestResolveDirectStrategyKeepsAChosenStrategy(t *testing.T) {
	s := setupRoutingStore(t)
	s.SingBox.DNS.Strategy = "prefer_ipv6"
	ResolveDirectStrategy(s, "ipv4_only")
	if s.SingBox.DNS.Strategy != "prefer_ipv6" || s.SingBox.DNS.Final != "public6" {
		t.Fatalf("chosen strategy not kept: %q/%q", s.SingBox.DNS.Strategy, s.SingBox.DNS.Final)
	}

	unread := setupRoutingStore(t)
	unread.SingBox.DNS.Strategy = "ipv4_only"
	if ResolveDirectStrategy(unread, "") || DirectStrategyResolved(unread) {
		t.Fatal("resolved without knowing the host's addresses")
	}
}

// Rules that differ only in their users become one rule naming every user, as
// shell-proxy compiled them; the route and DNS rules both follow.
func TestCompiledRulesMergeAcrossUsers(t *testing.T) {
	s := setupRoutingStore(t)
	add := func(user string, names []string, out string) {
		for _, name := range names {
			preset, ok := FindPreset(name)
			if !ok {
				t.Fatalf("preset %q", name)
			}
			if err := SetRule(s, user, PresetToRule(preset, user, out)); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("dhwang2", []string{"openai", "anthropic", "discord", "meta"}, "res1")
	add("dhwang1", []string{"openai", "anthropic", "linkedin"}, "res1")
	add("dhwang2", []string{"linkedin", "netflix"}, "res2")

	rules := CompiledUserRouteRules(s)
	type shape struct {
		outbound, users string
		sets            []string
	}
	got := []shape{}
	for _, rule := range rules {
		if len(rule.RuleSet) == 0 || !isGeositeRuleSet(rule.RuleSet) {
			continue
		}
		got = append(got, shape{rule.Outbound, strings.Join(rule.AuthUser, ","), rule.RuleSet})
	}
	// shell-proxy's order: outbound, then users, then the rule sets, each
	// rule's tags sorted.
	want := []shape{
		{"res1", "dhwang1", []string{"geosite-linkedin"}},
		{"res1", "dhwang1,dhwang2", []string{"geosite-anthropic", "geosite-openai"}},
		{"res1", "dhwang2", []string{"geosite-discord", "geosite-meta"}},
		{"res2", "dhwang2", []string{"geosite-linkedin", "geosite-netflix"}},
	}
	if len(got) != len(want) {
		t.Fatalf("geosite rules = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index].outbound != want[index].outbound || got[index].users != want[index].users ||
			strings.Join(got[index].sets, ",") != strings.Join(want[index].sets, ",") {
			t.Fatalf("rule %d = %+v, want %+v", index, got[index], want[index])
		}
	}

	// The DNS rules are derived from these, so the shared rule resolves once.
	// A chain's DNS rule needs the chain outbound to exist; direct always does.
	s = setupRoutingStore(t)
	add("dhwang1", []string{"openai"}, "direct")
	add("dhwang2", []string{"openai"}, "direct")
	Sync(s)
	shared := 0
	for _, rule := range s.SingBox.DNS.Rules {
		if strings.Join(rule.AuthUser, ",") == "dhwang1,dhwang2" {
			shared++
		}
	}
	if shared == 0 {
		t.Fatalf("no DNS rule carries both users: %+v", s.SingBox.DNS.Rules)
	}
}

// A merged rule is output only: the stored rules stay per user, so removing
// one user's rule leaves the other's in the compiled result.
func TestMergedRuleSplitsWhenOneUserLeaves(t *testing.T) {
	s := setupRoutingStore(t)
	preset, _ := FindPreset("openai")
	for _, user := range []string{"alice", "bob"} {
		if err := SetRule(s, user, PresetToRule(preset, user, "res1")); err != nil {
			t.Fatal(err)
		}
	}
	if rules := CompiledUserRouteRules(s); len(rules) == 0 || strings.Join(rules[0].AuthUser, ",") != "alice,bob" {
		t.Fatalf("not merged: %+v", rules)
	}
	ClearUser(s, "alice")
	for _, rule := range CompiledUserRouteRules(s) {
		if slices.Contains(rule.AuthUser, "alice") {
			t.Fatalf("alice outlived her rules: %+v", rule)
		}
	}
}
