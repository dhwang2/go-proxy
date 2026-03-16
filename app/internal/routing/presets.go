package routing

import "go-proxy/internal/store"

// Preset defines a named routing rule preset.
type Preset struct {
	Name     string
	RuleSets []string // geosite/geoip rule set tags
}

// BuiltinPresets returns the available routing presets.
func BuiltinPresets() []Preset {
	return []Preset{
		{Name: "OpenAI", RuleSets: []string{"geosite-openai"}},
		{Name: "Google", RuleSets: []string{"geosite-google"}},
		{Name: "Netflix", RuleSets: []string{"geosite-netflix"}},
		{Name: "Disney+", RuleSets: []string{"geosite-disney"}},
		{Name: "Telegram", RuleSets: []string{"geosite-telegram"}},
		{Name: "GitHub", RuleSets: []string{"geosite-github"}},
		{Name: "Apple", RuleSets: []string{"geosite-apple"}},
		{Name: "Microsoft", RuleSets: []string{"geosite-microsoft"}},
		{Name: "CN Direct", RuleSets: []string{"geosite-cn", "geoip-cn"}},
	}
}

// PresetToRule converts a preset to a UserRouteRule for a specific user and outbound.
func PresetToRule(preset Preset, userName, outbound string) store.UserRouteRule {
	return store.UserRouteRule{
		Action:   "route",
		Outbound: outbound,
		AuthUser: []string{userName},
		RuleSet:  preset.RuleSets,
	}
}
