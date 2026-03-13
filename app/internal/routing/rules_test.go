package routing

import (
	"testing"

	"github.com/dhwang2/go-proxy/internal/store"
)

func TestUpsertUserRulePreservesOtherUsersAndRebuildsDNS(t *testing.T) {
	st := newRoutingTestStore()
	st.Config.Route.Rules = []store.RouteRule{
		{Action: "route", Outbound: "res-socks", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-netflix"}},
		{Action: "route", Outbound: "direct", AuthUser: []string{"carol"}, RuleSet: []string{"geosite-cn"}},
		{Action: "route", Outbound: "block", RuleSet: []string{"geosite-category-ads-all"}},
	}
	st.Config.DNS.Rules = []store.DNSRule{
		{Action: "route", Server: "dns-final", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-netflix"}},
		{Action: "route", Server: "dns-custom", RuleSet: []string{"geosite-cn"}},
	}

	result, err := UpsertUserRule(st, "ALICE", "direct", []string{"geosite-openai, geosite-netflix", "geoip-us"})
	if err != nil {
		t.Fatalf("UpsertUserRule returned error: %v", err)
	}
	if !result.ConfigChanged {
		t.Fatalf("expected config change")
	}
	if result.RouteChanged != 2 {
		t.Fatalf("expected RouteChanged=2, got %d", result.RouteChanged)
	}
	if !st.ConfigDirty() {
		t.Fatalf("expected store config marked dirty")
	}

	if hasRouteUser(st.Config.Route.Rules, "alice") != 1 {
		t.Fatalf("expected exactly one route rule for alice after upsert")
	}
	if !hasRouteRule(st.Config.Route.Rules, "res-socks", []string{"bob"}, []string{"geosite-netflix"}) {
		t.Fatalf("expected bob rule to remain after alice upsert")
	}
	if !hasRouteRule(st.Config.Route.Rules, "direct", []string{"alice"}, []string{"geoip-us", "geosite-netflix", "geosite-openai"}) {
		t.Fatalf("expected new alice rule with normalized rule sets")
	}

	if !hasDNSRule(st.Config.DNS.Rules, "dns-custom", nil, []string{"geosite-cn"}) {
		t.Fatalf("expected unmanaged dns rule to be preserved")
	}
	if !hasDNSRule(st.Config.DNS.Rules, "dns-final", []string{"bob"}, []string{"geosite-netflix"}) {
		t.Fatalf("expected managed dns rule for bob")
	}
	if !hasDNSRule(st.Config.DNS.Rules, "dns-final", []string{"alice"}, []string{"geosite-netflix", "geosite-openai"}) {
		t.Fatalf("expected managed dns rule for alice")
	}
}

func TestClearUserRuleRemovesOnlyTargetUser(t *testing.T) {
	st := newRoutingTestStore()
	st.Config.Route.Rules = []store.RouteRule{
		{Action: "route", Outbound: "res-socks", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-netflix"}},
		{Action: "route", Outbound: "direct", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-openai"}},
		{Action: "route", Outbound: "block", RuleSet: []string{"geosite-category-ads-all"}},
	}
	st.Config.DNS.Rules = []store.DNSRule{
		{Action: "route", Server: "dns-final", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-netflix"}},
		{Action: "route", Server: "dns-custom", RuleSet: []string{"geosite-cn"}},
	}

	result, err := ClearUserRule(st, "alice")
	if err != nil {
		t.Fatalf("ClearUserRule returned error: %v", err)
	}
	if !result.ConfigChanged {
		t.Fatalf("expected config change")
	}
	if result.RouteChanged != 2 {
		t.Fatalf("expected RouteChanged=2, got %d", result.RouteChanged)
	}

	if hasRouteUser(st.Config.Route.Rules, "alice") != 0 {
		t.Fatalf("expected all alice route rules to be cleared")
	}
	if !hasRouteRule(st.Config.Route.Rules, "res-socks", []string{"bob"}, []string{"geosite-netflix"}) {
		t.Fatalf("expected bob route rule to remain")
	}
	if !hasDNSRule(st.Config.DNS.Rules, "dns-final", []string{"bob"}, []string{"geosite-netflix"}) {
		t.Fatalf("expected dns rule for bob after clear")
	}
}

func TestSyncDNSNoChangeWhenAlreadyAligned(t *testing.T) {
	st := newRoutingTestStore()
	st.Config.Route.Rules = []store.RouteRule{
		{Action: "route", Outbound: "direct", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-openai", "geoip-us"}},
		{Action: "route", Outbound: "res-socks", AuthUser: []string{"bob"}, RuleSet: []string{"geosite-netflix"}},
	}
	st.Config.DNS.Rules = []store.DNSRule{
		{Action: "route", Server: "dns-main", RuleSet: []string{"geosite-cn"}},
	}
	_ = rebuildManagedDNS(st)

	result, err := SyncDNS(st)
	if err != nil {
		t.Fatalf("SyncDNS returned error: %v", err)
	}
	if result.ConfigChanged {
		t.Fatalf("expected no config change")
	}
	if st.ConfigDirty() {
		t.Fatalf("expected store to remain clean")
	}
}

func TestTestUserCountsManagedRulesOnly(t *testing.T) {
	st := newRoutingTestStore()
	st.Config.Route.Rules = []store.RouteRule{
		{Action: "route", Outbound: "direct", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-openai"}},
		{Action: "route", Outbound: "res-socks", AuthUser: []string{"alice", "bob"}, RuleSet: []string{"geosite-netflix"}},
		{Action: "route", Outbound: "direct", AuthUser: []string{"alice"}, RuleSet: nil},
		{Action: "hijack", Outbound: "direct", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-fake"}},
	}

	byOutbound, total, err := TestUser(st, "alice")
	if err != nil {
		t.Fatalf("TestUser returned error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total managed route rules=2, got %d", total)
	}
	if byOutbound["direct"] != 1 || byOutbound["res-socks"] != 1 {
		t.Fatalf("unexpected outbound distribution: %#v", byOutbound)
	}
}

func newRoutingTestStore() *store.Store {
	return &store.Store{
		Config: &store.SingboxConfig{
			Outbounds: []store.Outbound{
				{Tag: "direct"},
				{Tag: "res-socks"},
				{Tag: "block"},
			},
			Route: store.Route{},
			DNS:   store.DNS{},
		},
		UserMeta: &store.UserMeta{
			Groups: map[string]store.Group{
				"alice": {},
				"bob":   {},
				"carol": {},
			},
		},
	}
}

func hasRouteUser(rules []store.RouteRule, user string) int {
	count := 0
	for _, r := range rules {
		if containsUser(r.AuthUser, user) {
			count++
		}
	}
	return count
}

func hasRouteRule(rules []store.RouteRule, outbound string, users []string, ruleSets []string) bool {
	for _, r := range rules {
		if r.Outbound != outbound {
			continue
		}
		if !sameStringSet(r.AuthUser, users) {
			continue
		}
		if !sameStringSet(r.RuleSet, ruleSets) {
			continue
		}
		return true
	}
	return false
}

func hasDNSRule(rules []store.DNSRule, server string, users []string, ruleSets []string) bool {
	for _, r := range rules {
		if r.Server != server {
			continue
		}
		if !sameStringSet(r.AuthUser, users) {
			continue
		}
		if !sameStringSet(r.RuleSet, ruleSets) {
			continue
		}
		return true
	}
	return false
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]int{}
	for _, v := range a {
		set[v]++
	}
	for _, v := range b {
		set[v]--
	}
	for _, n := range set {
		if n != 0 {
			return false
		}
	}
	return true
}
