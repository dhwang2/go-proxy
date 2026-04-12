package routing

import (
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

	SyncRouteRules(s)

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
