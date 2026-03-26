package routing

import (
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

func setupRoutingStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()

	config.SingBoxConfig = filepath.Join(dir, "conf", "sing-box.json")
	config.UserMetaFile = filepath.Join(dir, "user-management.json")
	config.UserRouteFile = filepath.Join(dir, "user-route-rules.json")
	config.UserTemplateFile = filepath.Join(dir, "user-route-templates.json")
	config.FirewallConfigFile = filepath.Join(dir, "firewall-ports.json")
	config.SnellConfigFile = filepath.Join(dir, "snell-v5.conf")
	config.SingBoxBin = "/nonexistent/sing-box"

	if err := os.MkdirAll(filepath.Dir(config.SingBoxConfig), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}

	s, err := store.Load()
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	return s
}

func TestReplaceUserRuleOutboundsKeepsClonedRuleAdjacent(t *testing.T) {
	s := setupRoutingStore(t)
	s.UserRoutes = []store.UserRouteRule{
		{Action: "route", Outbound: "old-a", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-openai"}},
		{Action: "route", Outbound: "old-b", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-google"}},
		{Action: "route", Outbound: "old-c", AuthUser: []string{"bob"}, RuleSet: []string{"geosite-twitter"}},
	}

	updated, err := ReplaceUserRuleOutbounds(s, "alice", []int{1}, "new-a")
	if err != nil {
		t.Fatalf("ReplaceUserRuleOutbounds error: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}
	if len(s.UserRoutes) != 4 {
		t.Fatalf("len(UserRoutes) = %d, want 4", len(s.UserRoutes))
	}
	if got := s.UserRoutes[0].AuthUser; len(got) != 1 || got[0] != "bob" {
		t.Fatalf("rule 0 auth_user = %#v, want [bob]", got)
	}
	if got := s.UserRoutes[1].AuthUser; len(got) != 1 || got[0] != "alice" {
		t.Fatalf("rule 1 auth_user = %#v, want [alice]", got)
	}
	if s.UserRoutes[1].Outbound != "new-a" {
		t.Fatalf("rule 1 outbound = %q, want new-a", s.UserRoutes[1].Outbound)
	}
	if s.UserRoutes[2].Outbound != "old-b" {
		t.Fatalf("rule 2 outbound = %q, want old-b", s.UserRoutes[2].Outbound)
	}
}

func TestSetRuleSplitsSharedRuleWithoutMovingPastLaterRules(t *testing.T) {
	s := setupRoutingStore(t)
	s.UserRoutes = []store.UserRouteRule{
		{Action: "route", Outbound: "old-a", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-openai"}},
		{Action: "route", Outbound: "old-b", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-google"}},
	}

	err := SetRule(s, "alice", store.UserRouteRule{
		Action:   "route",
		Outbound: "new-a",
		AuthUser: []string{"alice"},
		RuleSet:  []string{"geosite-openai"},
	})
	if err != nil {
		t.Fatalf("SetRule error: %v", err)
	}
	if len(s.UserRoutes) != 3 {
		t.Fatalf("len(UserRoutes) = %d, want 3", len(s.UserRoutes))
	}
	if got := s.UserRoutes[0].AuthUser; len(got) != 1 || got[0] != "bob" {
		t.Fatalf("rule 0 auth_user = %#v, want [bob]", got)
	}
	if got := s.UserRoutes[1].AuthUser; len(got) != 1 || got[0] != "alice" {
		t.Fatalf("rule 1 auth_user = %#v, want [alice]", got)
	}
	if s.UserRoutes[1].Outbound != "new-a" {
		t.Fatalf("rule 1 outbound = %q, want new-a", s.UserRoutes[1].Outbound)
	}
	if s.UserRoutes[2].Outbound != "old-b" {
		t.Fatalf("rule 2 outbound = %q, want old-b", s.UserRoutes[2].Outbound)
	}
}
