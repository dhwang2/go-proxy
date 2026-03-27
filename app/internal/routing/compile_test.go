package routing

import (
	"encoding/json"
	"testing"

	"go-proxy/internal/store"
)

func TestAddMenuPresetOptionsMatchShellProxyOrder(t *testing.T) {
	options := AddMenuPresetOptions()
	if len(options) != 25 {
		t.Fatalf("len(options) = %d, want 25", len(options))
	}
	if options[0].Key != "1" || options[0].Preset.Name != "openai" {
		t.Fatalf("options[0] = %#v", options[0])
	}
	if options[9].Key != "g" || options[9].Preset.Name != "discord" {
		t.Fatalf("options[9] = %#v", options[9])
	}
	if options[18].Key != "a" || options[18].Preset.Name != "ai-intl" {
		t.Fatalf("options[18] = %#v", options[18])
	}
	if options[24].Key != "r" || options[24].Preset.Name != "ads" {
		t.Fatalf("options[24] = %#v", options[24])
	}
}

func TestUserRouteLabelUsesPresetLabel(t *testing.T) {
	preset, ok := FindPreset("openai")
	if !ok {
		t.Fatal("FindPreset(openai) = false")
	}
	rule := store.UserRouteRule{
		Action:   "route",
		Outbound: "🐸 direct",
		AuthUser: []string{"alice"},
		RuleSet:  []string{"geosite-openai"},
	}
	if got := UserRouteLabel(rule); got != preset.Label {
		t.Fatalf("UserRouteLabel() = %q, want %q", got, preset.Label)
	}
	if got := OutboundLabel(rule.Outbound); got != "直连" {
		t.Fatalf("OutboundLabel() = %q, want 直连", got)
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

func TestCompileDNSRulesMatchesCompiledRouteRules(t *testing.T) {
	s := setupRoutingStore(t)
	preset, ok := FindPreset("google")
	if !ok {
		t.Fatal("FindPreset(google) = false")
	}
	s.UserRoutes = []store.UserRouteRule{
		PresetToRule(preset, "alice", "proxy-a"),
	}

	dnsRules := CompileDNSRules(s, map[string]string{"proxy-a": "dns-proxy"}, "ipv4_only")
	if len(dnsRules) != 2 {
		t.Fatalf("len(dnsRules) = %d, want 2", len(dnsRules))
	}
	if got := dnsRules[0].RuleSet; len(got) != 1 || got[0] != "geosite-google" {
		t.Fatalf("dnsRules[0].RuleSet = %#v", got)
	}
	if got := dnsRules[1].RuleSet; len(got) != 1 || got[0] != "geoip-google" {
		t.Fatalf("dnsRules[1].RuleSet = %#v", got)
	}
}

func TestCompiledUserRouteRulesRestoresMissingPresetGeoIPRuleSet(t *testing.T) {
	s := setupRoutingStore(t)
	var kept []json.RawMessage
	for _, raw := range s.SingBox.Route.RuleSet {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err == nil && item.Tag == "geoip-ai" {
			continue
		}
		kept = append(kept, raw)
	}
	s.SingBox.Route.RuleSet = kept
	s.UserRoutes = []store.UserRouteRule{
		{
			Action:   "route",
			Outbound: "proxy-a",
			AuthUser: []string{"alice"},
			RuleSet:  []string{"geosite-category-ai-!cn"},
		},
	}

	rules := CompiledUserRouteRules(s)
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(rules))
	}
	if got := rules[0].RuleSet; len(got) != 1 || got[0] != "geosite-category-ai-!cn" {
		t.Fatalf("rules[0].RuleSet = %#v", got)
	}
	if got := rules[1].RuleSet; len(got) != 1 || got[0] != "geoip-ai" {
		t.Fatalf("rules[1].RuleSet = %#v", got)
	}
}
