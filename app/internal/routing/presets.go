package routing

import "go-proxy/internal/store"

// PresetCategory groups presets for menu display.
type PresetCategory struct {
	Name    string
	Presets []Preset
}

// Preset defines a named routing rule preset.
type Preset struct {
	Name     string   // internal key (e.g., "openai")
	Label    string   // display label (e.g., "OpenAI/ChatGPT")
	RuleSets []string // geosite/geoip rule set tags
}

// BuiltinPresets returns the available routing presets grouped by category.
func BuiltinPresets() []Preset {
	var all []Preset
	for _, cat := range PresetCategories() {
		all = append(all, cat.Presets...)
	}
	return all
}

// PresetCategories returns presets organized by category for menu display.
func PresetCategories() []PresetCategory {
	return []PresetCategory{
		{
			Name: "AI",
			Presets: []Preset{
				{Name: "openai", Label: "OpenAI/ChatGPT", RuleSets: []string{"geosite-openai"}},
				{Name: "anthropic", Label: "Anthropic/Claude", RuleSets: []string{"geosite-anthropic"}},
				{Name: "ai-intl", Label: "AI服务(国际)", RuleSets: []string{"geosite-category-ai-!cn"}},
				{Name: "xai", Label: "xAI/Grok", RuleSets: []string{"geosite-xai"}},
			},
		},
		{
			Name: "Content",
			Presets: []Preset{
				{Name: "google", Label: "Google", RuleSets: []string{"geosite-google", "geoip-google"}},
				{Name: "youtube", Label: "YouTube", RuleSets: []string{"geosite-youtube"}},
				{Name: "netflix", Label: "Netflix", RuleSets: []string{"geosite-netflix", "geoip-netflix"}},
				{Name: "disney", Label: "Disney+", RuleSets: []string{"geosite-disney"}},
				{Name: "mytvsuper", Label: "MyTVSuper", RuleSets: []string{"geosite-mytvsuper"}},
				{Name: "spotify", Label: "Spotify", RuleSets: []string{"geosite-spotify"}},
				{Name: "tiktok", Label: "TikTok", RuleSets: []string{"geosite-tiktok"}},
				{Name: "github", Label: "GitHub", RuleSets: []string{"geosite-github"}},
			},
		},
		{
			Name: "Social",
			Presets: []Preset{
				{Name: "telegram", Label: "Telegram", RuleSets: []string{"geosite-telegram", "geoip-telegram"}},
				{Name: "twitter", Label: "Twitter/X", RuleSets: []string{"geosite-twitter", "geoip-twitter"}},
				{Name: "whatsapp", Label: "WhatsApp", RuleSets: []string{"geosite-whatsapp"}},
				{Name: "facebook", Label: "Facebook", RuleSets: []string{"geosite-facebook", "geoip-facebook"}},
				{Name: "discord", Label: "Discord", RuleSets: []string{"geosite-discord"}},
				{Name: "instagram", Label: "Instagram", RuleSets: []string{"geosite-instagram"}},
				{Name: "reddit", Label: "Reddit", RuleSets: []string{"geosite-reddit"}},
				{Name: "linkedin", Label: "LinkedIn", RuleSets: []string{"geosite-linkedin"}},
				{Name: "meta", Label: "Meta", RuleSets: []string{"geosite-meta"}},
				{Name: "messenger", Label: "Messenger", RuleSets: []string{"geosite-messenger"}},
			},
		},
		{
			Name: "Services",
			Presets: []Preset{
				{Name: "paypal", Label: "PayPal", RuleSets: []string{"geosite-paypal"}},
				{Name: "microsoft", Label: "Microsoft", RuleSets: []string{"geosite-microsoft"}},
			},
		},
		{
			Name: "Special",
			Presets: []Preset{
				{Name: "ads", Label: "广告屏蔽", RuleSets: []string{"geosite-category-ads-all"}},
				{Name: "custom", Label: "自定义", RuleSets: nil},
			},
		},
	}
}

// FindPreset looks up a preset by name.
func FindPreset(name string) (Preset, bool) {
	for _, p := range BuiltinPresets() {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
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
