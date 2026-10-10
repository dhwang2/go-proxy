package routing

import (
	"slices"
	"strings"

	"go-proxy/internal/store"
)

// Kind groups presets for colour: AI services, platforms, social networks and
// blocking.
type Kind int

const (
	KindAI Kind = iota
	KindPlatform
	KindSocial
	KindBlock
)

// Preset is a routing preset: its menu index, its name and label, and the
// rule sets a rule made from it matches.
type Preset struct {
	Symbol   string
	Name     string
	Label    string
	Kind     Kind
	RuleSets []string
}

// presets is the menu, in menu order. The indexes are shell-proxy's rule menu
// numbering, 1-9 first and then letters, so an index learned there selects
// the same preset here; spacex was added after the port.
var presets = []Preset{
	{"1", "openai", "OpenAI/ChatGPT", KindAI, []string{"geosite-openai"}},
	{"2", "anthropic", "Anthropic/Claude", KindAI, []string{"geosite-anthropic"}},
	{"3", "google", "Google", KindPlatform, []string{"geosite-google", "geoip-google"}},
	{"4", "youtube", "YouTube", KindPlatform, []string{"geosite-youtube"}},
	{"5", "telegram", "Telegram", KindSocial, []string{"geosite-telegram", "geoip-telegram"}},
	{"6", "twitter", "Twitter/X", KindSocial, []string{"geosite-twitter", "geoip-twitter"}},
	{"7", "whatsapp", "WhatsApp", KindSocial, []string{"geosite-whatsapp"}},
	{"8", "facebook", "Facebook", KindSocial, []string{"geosite-facebook", "geoip-facebook"}},
	{"9", "github", "GitHub", KindPlatform, []string{"geosite-github"}},
	{"g", "discord", "Discord", KindSocial, []string{"geosite-discord"}},
	{"h", "instagram", "Instagram", KindSocial, []string{"geosite-instagram"}},
	{"i", "reddit", "Reddit", KindSocial, []string{"geosite-reddit"}},
	{"j", "xai", "xAI/Grok", KindAI, []string{"geosite-xai"}},
	{"k", "microsoft", "Microsoft", KindPlatform, []string{"geosite-microsoft"}},
	{"l", "linkedin", "LinkedIn", KindSocial, []string{"geosite-linkedin"}},
	{"m", "paypal", "PayPal", KindPlatform, []string{"geosite-paypal"}},
	{"n", "meta", "Meta", KindSocial, []string{"geosite-meta"}},
	{"o", "messenger", "Messenger", KindSocial, []string{"geosite-messenger"}},
	{"a", "ai-intl", "AI (Intl)", KindAI, []string{"geosite-category-ai-!cn", "geoip-ai"}},
	{"b", "netflix", "Netflix", KindPlatform, []string{"geosite-netflix", "geoip-netflix"}},
	{"d", "disney", "Disney+", KindPlatform, []string{"geosite-disney"}},
	{"e", "mytvsuper", "MyTVSuper", KindPlatform, []string{"geosite-mytvsuper"}},
	{"s", "spotify", "Spotify", KindPlatform, []string{"geosite-spotify"}},
	{"t", "tiktok", "TikTok", KindPlatform, []string{"geosite-tiktok"}},
	{"x", "spacex", "SpaceX/Starlink", KindPlatform, []string{"geosite-spacex"}},
	{"r", "ads", "Ad blocking", KindBlock, []string{"geosite-category-ads-all"}},
}

// Presets lists the presets in menu order.
func Presets() []Preset { return presets }

func findPreset(match func(Preset) bool) (Preset, bool) {
	index := slices.IndexFunc(presets, match)
	if index < 0 {
		return Preset{}, false
	}
	return presets[index], true
}

// FindPreset looks up a preset by name.
func FindPreset(name string) (Preset, bool) {
	return findPreset(func(p Preset) bool { return p.Name == name })
}

// ResolvePresetSelector maps a menu index to its preset. The menu, the rule
// listing and every --rules value use the one numbering; letters are matched
// without regard to case.
func ResolvePresetSelector(selector string) (Preset, bool) {
	selector = strings.ToLower(strings.TrimSpace(selector))
	return findPreset(func(p Preset) bool { return p.Symbol == selector })
}

// PresetOf names the preset a stored rule was made from. The file keeps
// sing-box's field names, so a rule carries its preset's rule sets and is
// recognised by them; a rule matching no preset is not one gproxy made.
func PresetOf(rule store.UserRouteRule) (Preset, bool) {
	return findPreset(func(p Preset) bool { return slices.Equal(rule.RuleSet, p.RuleSets) })
}

// PresetRank is a preset's position in the menu, for ordering a listing the
// way the menu reads.
func PresetRank(name string) int {
	return slices.IndexFunc(presets, func(p Preset) bool { return p.Name == name })
}

// PresetToRule is the stored rule sending one user's preset traffic to outbound.
func PresetToRule(preset Preset, userName, outbound string) store.UserRouteRule {
	return store.UserRouteRule{
		Outbound: outbound,
		AuthUser: []string{userName},
		RuleSet:  slices.Clone(preset.RuleSets),
	}
}
