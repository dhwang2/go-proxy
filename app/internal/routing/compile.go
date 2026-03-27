package routing

import (
	"encoding/json"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

type MenuPresetOption struct {
	Key    string
	Preset Preset
}

func AddMenuPresetOptions() []MenuPresetOption {
	order := []struct {
		key  string
		name string
	}{
		{"1", "openai"},
		{"2", "anthropic"},
		{"3", "google"},
		{"4", "youtube"},
		{"5", "telegram"},
		{"6", "twitter"},
		{"7", "whatsapp"},
		{"8", "facebook"},
		{"9", "github"},
		{"g", "discord"},
		{"h", "instagram"},
		{"i", "reddit"},
		{"j", "xai"},
		{"k", "microsoft"},
		{"l", "linkedin"},
		{"m", "paypal"},
		{"n", "meta"},
		{"o", "messenger"},
		{"a", "ai-intl"},
		{"b", "netflix"},
		{"d", "disney"},
		{"e", "mytvsuper"},
		{"s", "spotify"},
		{"t", "tiktok"},
		{"r", "ads"},
	}
	options := make([]MenuPresetOption, 0, len(order))
	for _, item := range order {
		preset, ok := FindPreset(item.name)
		if !ok {
			continue
		}
		options = append(options, MenuPresetOption{Key: item.key, Preset: preset})
	}
	return options
}

func CompiledUserRouteRules(s *store.Store) []store.RouteRule {
	// Resolve presets once for all rules.
	presets := resolvePresets(s.UserRoutes)
	ensureRequiredRuleSetCatalog(s)
	available := availableRuleSetTags(s)
	rules := make([]store.RouteRule, 0, len(s.UserRoutes))
	for i, rule := range s.UserRoutes {
		if presets[i] != nil {
			rules = append(rules, compilePresetRule(rule, *presets[i], available)...)
		} else {
			rules = append(rules, compileGenericRule(rule)...)
		}
	}
	return mergeRouteRulesByOutbound(rules)
}

// resolvePresets maps each UserRouteRule to its matching preset (or nil).
func resolvePresets(rules []store.UserRouteRule) []*Preset {
	result := make([]*Preset, len(rules))
	for i, rule := range rules {
		if p, ok := presetForRule(rule); ok {
			result[i] = &p
		}
	}
	return result
}

func CompileDNSRules(s *store.Store, outboundToDNS map[string]string, defaultStrategy string) []store.DNSRule {
	return mergeDNSRulesByServer(dnsRulesFromRouteRules(CompiledUserRouteRules(s), outboundToDNS, defaultStrategy))
}

func UserRouteLabel(rule store.UserRouteRule) string {
	if preset, ok := presetForRule(rule); ok {
		return preset.Label
	}
	for _, values := range [][]string{rule.Domain, rule.DomainSuffix, rule.DomainKeyword, rule.DomainRegex, rule.IPCIDR, rule.RuleSet} {
		if len(values) > 0 {
			return strings.Join(values, ",")
		}
	}
	return "自定义"
}

func OutboundLabel(outbound string) string {
	switch outbound {
	case "direct", "🐸 direct":
		return "直连"
	default:
		return outbound
	}
}

func presetForRule(rule store.UserRouteRule) (Preset, bool) {
	if len(rule.Domain) > 0 || len(rule.DomainKeyword) > 0 || len(rule.DomainRegex) > 0 || len(rule.IPCIDR) > 0 {
		return Preset{}, false
	}
	for _, preset := range BuiltinPresets() {
		if preset.Name == "custom" {
			continue
		}
		if ruleMatchesPreset(rule, preset) {
			return preset, true
		}
	}
	return Preset{}, false
}

// ruleMatchesPreset uses subset matching: a stored rule with ["geosite-google"]
// matches the "google" preset (which has ["geosite-google","geoip-google"]).
// This is intentional — old rules stored before geoip tags were added still match
// their preset, and compilePresetRule expands them to full coverage.
func ruleMatchesPreset(rule store.UserRouteRule, preset Preset) bool {
	if len(rule.DomainSuffix) > 0 && !sameStringSlice(rule.DomainSuffix, preset.FallbackDomains) {
		return false
	}
	if len(rule.RuleSet) == 0 {
		return false
	}
	for _, tag := range rule.RuleSet {
		found := false
		for _, presetTag := range preset.RuleSets {
			if tag == presetTag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func availableRuleSetTags(s *store.Store) map[string]bool {
	tags := make(map[string]bool)
	if s == nil || s.SingBox == nil || s.SingBox.Route == nil {
		return tags
	}
	for _, raw := range s.SingBox.Route.RuleSet {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err == nil && item.Tag != "" {
			tags[item.Tag] = true
		}
	}
	return tags
}

func ensureRequiredRuleSetCatalog(s *store.Store) {
	if s == nil || s.SingBox == nil {
		return
	}
	if s.SingBox.Route == nil {
		s.SingBox.Route = &store.RouteConfig{}
	}

	type entry struct {
		tag string
		raw json.RawMessage
	}

	current := make([]entry, 0, len(s.SingBox.Route.RuleSet))
	seen := make(map[string]bool, len(s.SingBox.Route.RuleSet))
	for _, raw := range s.SingBox.Route.RuleSet {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			current = append(current, entry{raw: append(json.RawMessage(nil), raw...)})
			continue
		}
		current = append(current, entry{tag: item.Tag, raw: append(json.RawMessage(nil), raw...)})
		if item.Tag != "" {
			seen[item.Tag] = true
		}
	}

	catalog := make(map[string]json.RawMessage)
	for _, item := range config.DefaultRuleSetCatalog() {
		tag, _ := item["tag"].(string)
		if tag == "" {
			continue
		}
		raw, err := json.Marshal(item)
		if err != nil {
			continue
		}
		catalog[tag] = raw
	}

	changed := false
	for _, tag := range requiredRuleSetTags(s.UserRoutes) {
		if seen[tag] {
			continue
		}
		raw, ok := catalog[tag]
		if !ok {
			continue
		}
		current = append(current, entry{tag: tag, raw: raw})
		seen[tag] = true
		changed = true
	}
	if !changed {
		return
	}

	var geosite, other, geoip []json.RawMessage
	for _, item := range current {
		switch {
		case strings.HasPrefix(item.tag, "geosite-"):
			geosite = append(geosite, item.raw)
		case strings.HasPrefix(item.tag, "geoip-"):
			geoip = append(geoip, item.raw)
		default:
			other = append(other, item.raw)
		}
	}
	s.SingBox.Route.RuleSet = append(append(geosite, other...), geoip...)
}

func requiredRuleSetTags(rules []store.UserRouteRule) []string {
	var tags []string
	for _, rule := range rules {
		if preset, ok := presetForRule(rule); ok {
			tags = append(tags, preset.RuleSets...)
			continue
		}
		tags = append(tags, rule.RuleSet...)
	}
	return uniqueStrings(tags)
}

func compilePresetRule(rule store.UserRouteRule, preset Preset, available map[string]bool) []store.RouteRule {
	base := userRouteToRouteRule(rule)
	var compiled []store.RouteRule
	var geosite, geoip []string
	for _, tag := range preset.RuleSets {
		switch {
		case strings.HasPrefix(tag, "geosite-") && available[tag]:
			geosite = append(geosite, tag)
		case strings.HasPrefix(tag, "geoip-") && available[tag]:
			geoip = append(geoip, tag)
		}
	}
	if len(geosite) > 0 {
		rr := base
		rr.RuleSet = append([]string(nil), geosite...)
		rr.DomainSuffix = nil
		compiled = append(compiled, rr)
	}
	if len(geoip) > 0 {
		rr := base
		rr.RuleSet = append([]string(nil), geoip...)
		rr.DomainSuffix = nil
		compiled = append(compiled, rr)
	}
	if len(compiled) == 0 && len(preset.FallbackDomains) > 0 {
		rr := base
		rr.RuleSet = nil
		rr.DomainSuffix = append([]string(nil), preset.FallbackDomains...)
		compiled = append(compiled, rr)
	}
	return compiled
}

func compileGenericRule(rule store.UserRouteRule) []store.RouteRule {
	if len(rule.RuleSet) == 0 {
		return []store.RouteRule{userRouteToRouteRule(rule)}
	}
	var geosite, geoip, other []string
	for _, tag := range rule.RuleSet {
		switch {
		case strings.HasPrefix(tag, "geosite-"):
			geosite = append(geosite, tag)
		case strings.HasPrefix(tag, "geoip-"):
			geoip = append(geoip, tag)
		default:
			other = append(other, tag)
		}
	}
	base := userRouteToRouteRule(rule)
	var compiled []store.RouteRule
	for _, ruleSet := range [][]string{geosite, geoip, other} {
		if len(ruleSet) == 0 {
			continue
		}
		rr := base
		rr.RuleSet = append([]string(nil), ruleSet...)
		compiled = append(compiled, rr)
	}
	return compiled
}

func userRouteToRouteRule(rule store.UserRouteRule) store.RouteRule {
	action := rule.Action
	if action == "" {
		action = "route"
	}
	return store.RouteRule{
		Action:        action,
		Outbound:      rule.Outbound,
		AuthUser:      append([]string(nil), rule.AuthUser...),
		RuleSet:       append([]string(nil), rule.RuleSet...),
		Domain:        append([]string(nil), rule.Domain...),
		DomainSuffix:  append([]string(nil), rule.DomainSuffix...),
		DomainKeyword: append([]string(nil), rule.DomainKeyword...),
		DomainRegex:   append([]string(nil), rule.DomainRegex...),
		IPCIDR:        append([]string(nil), rule.IPCIDR...),
	}
}

func dnsRulesFromRouteRules(routeRules []store.RouteRule, outboundToDNS map[string]string, defaultStrategy string) []store.DNSRule {
	rules := make([]store.DNSRule, 0, len(routeRules))
	for _, rule := range routeRules {
		if len(rule.AuthUser) == 0 {
			continue
		}
		server, ok := outboundToDNS[rule.Outbound]
		if !ok {
			continue
		}
		dnsRule := store.DNSRule{
			Action:        "route",
			Server:        server,
			Strategy:      defaultStrategy,
			AuthUser:      append([]string(nil), rule.AuthUser...),
			RuleSet:       append([]string(nil), rule.RuleSet...),
			Domain:        append([]string(nil), rule.Domain...),
			DomainSuffix:  append([]string(nil), rule.DomainSuffix...),
			DomainKeyword: append([]string(nil), rule.DomainKeyword...),
			DomainRegex:   append([]string(nil), rule.DomainRegex...),
		}
		if len(dnsRule.RuleSet) == 0 &&
			len(dnsRule.Domain) == 0 &&
			len(dnsRule.DomainSuffix) == 0 &&
			len(dnsRule.DomainKeyword) == 0 &&
			len(dnsRule.DomainRegex) == 0 {
			continue
		}
		rules = append(rules, dnsRule)
	}
	return rules
}
