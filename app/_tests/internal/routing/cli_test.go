package routing

import (
	"encoding/json"
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
					if rule.Server != ChainDNSTag("relay-a") || rule.Strategy != strategy {
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

func TestDomainEvaluationDoesNotClaimRuleSetMatch(t *testing.T) {
	s := setupRoutingStore(t)
	s.UserRoutes = []store.UserRouteRule{{AuthUser: []string{"alice"}, Outbound: "relay-a", RuleSet: []string{"geosite-openai"}}}
	result := TestDomain(s, "alice", "unrelated.example")
	if len(result.MatchedRules) != 0 || len(result.Unresolved) != 1 {
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
