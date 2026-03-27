package routing

import "go-proxy/internal/store"

// PresetCategory groups presets for menu display.
type PresetCategory struct {
	Name    string
	Presets []Preset
}

// Preset defines a named routing rule preset.
type Preset struct {
	Name            string   // internal key (e.g., "openai")
	Label           string   // display label (e.g., "OpenAI/ChatGPT")
	RuleSets        []string // geosite/geoip rule set tags
	FallbackDomains []string // domain_suffix fallback when rule sets are unavailable
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
// Preset set matches shell-proxy routing_preset_meta() (26 presets).
// Within each category, relative order follows shell-proxy numbering.
func PresetCategories() []PresetCategory {
	return []PresetCategory{
		{
			Name: "AI",
			Presets: []Preset{
				{
					Name:            "openai",
					Label:           "OpenAI/ChatGPT",
					RuleSets:        []string{"geosite-openai"},
					FallbackDomains: []string{"openai.com", "chatgpt.com", "oaistatic.com"},
				},
				{
					Name:            "anthropic",
					Label:           "Anthropic/Claude",
					RuleSets:        []string{"geosite-anthropic"},
					FallbackDomains: []string{"anthropic.com", "claude.ai"},
				},
				{
					Name:            "xai",
					Label:           "xAI/Grok",
					RuleSets:        []string{"geosite-xai"},
					FallbackDomains: []string{"x.ai", "grok.com"},
				},
				{
					Name:            "ai-intl",
					Label:           "AI服务(国际)",
					RuleSets:        []string{"geosite-category-ai-!cn", "geoip-ai"},
					FallbackDomains: []string{"openai.com", "anthropic.com", "claude.ai", "chatgpt.com"},
				},
			},
		},
		{
			Name: "Content",
			Presets: []Preset{
				{
					Name:            "google",
					Label:           "Google",
					RuleSets:        []string{"geosite-google", "geoip-google"},
					FallbackDomains: []string{"google.com", "gstatic.com", "googleapis.com", "googlevideo.com"},
				},
				{
					Name:            "netflix",
					Label:           "Netflix",
					RuleSets:        []string{"geosite-netflix", "geoip-netflix"},
					FallbackDomains: []string{"netflix.com", "nflxvideo.net", "nflximg.net", "nflxso.net", "nflxext.com"},
				},
				{
					Name:            "disney",
					Label:           "Disney+",
					RuleSets:        []string{"geosite-disney"},
					FallbackDomains: []string{"disneyplus.com", "dssott.com", "bamgrid.com", "disney.com"},
				},
				{
					Name:            "mytvsuper",
					Label:           "MyTVSuper",
					RuleSets:        []string{"geosite-mytvsuper"},
					FallbackDomains: []string{"mytvsuper.com", "tvb.com"},
				},
				{
					Name:            "youtube",
					Label:           "YouTube",
					RuleSets:        []string{"geosite-youtube"},
					FallbackDomains: []string{"youtube.com", "youtu.be", "googlevideo.com"},
				},
				{
					Name:            "spotify",
					Label:           "Spotify",
					RuleSets:        []string{"geosite-spotify"},
					FallbackDomains: []string{"spotify.com", "scdn.co", "spotifycdn.com"},
				},
				{
					Name:            "tiktok",
					Label:           "TikTok",
					RuleSets:        []string{"geosite-tiktok"},
					FallbackDomains: []string{"tiktok.com", "tiktokv.com", "tiktokcdn.com"},
				},
				{
					Name:            "github",
					Label:           "GitHub",
					RuleSets:        []string{"geosite-github"},
					FallbackDomains: []string{"github.com", "githubusercontent.com"},
				},
			},
		},
		{
			Name: "Social",
			Presets: []Preset{
				{
					Name:            "telegram",
					Label:           "Telegram",
					RuleSets:        []string{"geosite-telegram", "geoip-telegram"},
					FallbackDomains: []string{"telegram.org", "t.me"},
				},
				{
					Name:            "twitter",
					Label:           "Twitter/X",
					RuleSets:        []string{"geosite-twitter", "geoip-twitter"},
					FallbackDomains: []string{"twitter.com", "x.com", "twimg.com"},
				},
				{
					Name:            "whatsapp",
					Label:           "WhatsApp",
					RuleSets:        []string{"geosite-whatsapp"},
					FallbackDomains: []string{"whatsapp.com", "whatsapp.net"},
				},
				{
					Name:            "facebook",
					Label:           "Facebook",
					RuleSets:        []string{"geosite-facebook", "geoip-facebook"},
					FallbackDomains: []string{"facebook.com", "fbcdn.net", "messenger.com"},
				},
				{
					Name:            "discord",
					Label:           "Discord",
					RuleSets:        []string{"geosite-discord"},
					FallbackDomains: []string{"discord.com", "discord.gg", "discordapp.com", "discordapp.net"},
				},
				{
					Name:            "instagram",
					Label:           "Instagram",
					RuleSets:        []string{"geosite-instagram"},
					FallbackDomains: []string{"instagram.com", "cdninstagram.com"},
				},
				{
					Name:            "reddit",
					Label:           "Reddit",
					RuleSets:        []string{"geosite-reddit"},
					FallbackDomains: []string{"reddit.com", "redd.it", "redditmedia.com"},
				},
				{
					Name:            "linkedin",
					Label:           "LinkedIn",
					RuleSets:        []string{"geosite-linkedin"},
					FallbackDomains: []string{"linkedin.com", "licdn.com"},
				},
				{
					Name:            "meta",
					Label:           "Meta",
					RuleSets:        []string{"geosite-meta"},
					FallbackDomains: []string{"meta.com", "fb.com"},
				},
				{
					Name:            "messenger",
					Label:           "Messenger",
					RuleSets:        []string{"geosite-messenger"},
					FallbackDomains: []string{"messenger.com", "m.me"},
				},
			},
		},
		{
			Name: "Services",
			Presets: []Preset{
				{
					Name:            "paypal",
					Label:           "PayPal",
					RuleSets:        []string{"geosite-paypal"},
					FallbackDomains: []string{"paypal.com", "paypalobjects.com"},
				},
				{
					Name:            "microsoft",
					Label:           "Microsoft",
					RuleSets:        []string{"geosite-microsoft"},
					FallbackDomains: []string{"microsoft.com", "live.com", "outlook.com", "office.com", "msauth.net", "msftauth.net"},
				},
			},
		},
		{
			Name: "Special",
			Presets: []Preset{
				{
					Name:            "ads",
					Label:           "广告屏蔽",
					RuleSets:        []string{"geosite-category-ads-all"},
					FallbackDomains: []string{"doubleclick.net", "googlesyndication.com", "googleadservices.com", "adservice.google.com", "googletagmanager.com"},
				},
				{Name: "custom", Label: "自定义", RuleSets: nil},
			},
		},
	}
}

// AddMenuPresets returns presets in the shell-proxy add-rule menu order.
func AddMenuPresets() []Preset {
	options := AddMenuPresetOptions()
	result := make([]Preset, 0, len(options))
	for _, option := range options {
		result = append(result, option.Preset)
	}
	return result
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
		Action:       "route",
		Outbound:     outbound,
		AuthUser:     []string{userName},
		RuleSet:      preset.RuleSets,
		DomainSuffix: preset.FallbackDomains,
	}
}
