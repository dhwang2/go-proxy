package routing

import (
	"regexp"
	"strings"

	"go-proxy/internal/store"
)

// PresetCategory groups presets for menu display.
type PresetCategory struct {
	Name    string
	Presets []Preset
}

// Preset defines a named routing rule preset.
type Preset struct {
	Name            string   `json:"name"`
	Label           string   `json:"label"`
	RuleSets        []string `json:"rule_sets"`
	FallbackDomains []string `json:"fallback_domains"`
}

// BuiltinPresets returns the available routing presets grouped by category.
var builtinPresets = func() []Preset {
	var all []Preset
	for _, cat := range PresetCategories() {
		all = append(all, cat.Presets...)
	}
	return all
}()

func BuiltinPresets() []Preset { return builtinPresets }

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
					Label:           "AI (Intl)",
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
					Label:           "Ad blocking",
					RuleSets:        []string{"geosite-category-ads-all"},
					FallbackDomains: []string{"doubleclick.net", "googlesyndication.com", "googleadservices.com", "adservice.google.com", "googletagmanager.com"},
				},
				{Name: "custom", Label: "Custom", RuleSets: nil},
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
		Action:       "route",
		Outbound:     outbound,
		AuthUser:     []string{userName},
		RuleSet:      preset.RuleSets,
		DomainSuffix: preset.FallbackDomains,
	}
}

// PresetChoice is one entry of the numbered preset menu.
type PresetChoice struct {
	Symbol string
	Preset Preset
}

// presetSymbols is shell-proxy's rule menu numbering, kept so an index a
// reader learned there selects the same preset here: 1-9 first, then letters.
var presetSymbols = []struct{ symbol, name string }{
	{"1", "openai"}, {"2", "anthropic"}, {"3", "google"}, {"4", "youtube"},
	{"5", "telegram"}, {"6", "twitter"}, {"7", "whatsapp"}, {"8", "facebook"},
	{"9", "github"}, {"g", "discord"}, {"h", "instagram"}, {"i", "reddit"},
	{"j", "xai"}, {"k", "microsoft"}, {"l", "linkedin"}, {"m", "paypal"},
	{"n", "meta"}, {"o", "messenger"}, {"a", "ai-intl"}, {"b", "netflix"},
	{"d", "disney"}, {"e", "mytvsuper"}, {"s", "spotify"}, {"t", "tiktok"},
	{"r", "ads"},
}

// PresetMenu lists the selectable presets in menu order with their index.
func PresetMenu() []PresetChoice {
	menu := make([]PresetChoice, 0, len(presetSymbols))
	for _, entry := range presetSymbols {
		if preset, ok := FindPreset(entry.name); ok {
			menu = append(menu, PresetChoice{Symbol: entry.symbol, Preset: preset})
		}
	}
	return menu
}

// ResolvePresetSelector maps a menu index to the preset name. Only the index
// is accepted: the menu, the rule listing and every --rules value
// use the one numbering. Letters are matched without regard to case.
func ResolvePresetSelector(selector string) (string, bool) {
	selector = strings.ToLower(strings.TrimSpace(selector))
	for _, entry := range presetSymbols {
		if entry.symbol == selector {
			return entry.name, true
		}
	}
	return "", false
}

// UserRoutePreset names the preset a stored rule was made from, or "" for a
// custom rule.
func UserRoutePreset(rule store.UserRouteRule) string {
	if preset, ok := presetForRule(rule); ok {
		return preset.Name
	}
	return ""
}

// PresetSymbol is a preset's menu index, "" for a name the menu does not hold.
func PresetSymbol(name string) string {
	for _, entry := range presetSymbols {
		if entry.name == name {
			return entry.symbol
		}
	}
	return ""
}

// presetRank is a preset's position in the menu, for ordering a listing the
// way the menu reads; a rule no preset made sorts after every preset.
func PresetRank(name string) int {
	for index, entry := range presetSymbols {
		if entry.name == name {
			return index
		}
	}
	return len(presetSymbols)
}

// ValidRuleSelector reports whether a --rules value can name a rule: a menu
// index, or cN for the Nth rule no preset made.
func ValidRuleSelector(selector string) bool {
	selector = strings.ToLower(strings.TrimSpace(selector))
	if _, ok := ResolvePresetSelector(selector); ok {
		return true
	}
	return customSelector.MatchString(selector)
}

var customSelector = regexp.MustCompile(`^c[1-9][0-9]*$`)
