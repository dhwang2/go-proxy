package routing

import (
	"fmt"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

// PresetRule defines a named group of rule sets.
type PresetRule struct {
	Name     string
	RuleSets []string
}

var presets = []PresetRule{
	{
		Name:     "china-direct",
		RuleSets: []string{"geosite-cn", "geoip-cn"},
	},
	{
		Name:     "ad-block",
		RuleSets: []string{"geosite-category-ads-all"},
	},
	{
		Name:     "streaming",
		RuleSets: []string{"geosite-netflix", "geosite-disney", "geosite-youtube"},
	},
}

// AvailablePresets returns known preset rule groups.
func AvailablePresets() []PresetRule {
	out := make([]PresetRule, len(presets))
	copy(out, presets)
	return out
}

// ApplyPreset adds a preset's rule sets to a user's routing rule for a given outbound.
func ApplyPreset(st *store.Store, user, outbound, presetName string) (MutationResult, error) {
	if st == nil || st.Config == nil || st.UserMeta == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	user = normalizeName(user)
	outbound = strings.TrimSpace(outbound)
	presetName = strings.TrimSpace(strings.ToLower(presetName))
	if user == "" || outbound == "" || presetName == "" {
		return MutationResult{}, fmt.Errorf("usage: routing preset <user> <outbound> <preset>")
	}

	var matched *PresetRule
	for i := range presets {
		if presets[i].Name == presetName {
			matched = &presets[i]
			break
		}
	}
	if matched == nil {
		return MutationResult{}, fmt.Errorf("unknown preset: %s", presetName)
	}

	return UpsertUserRule(st, user, outbound, matched.RuleSets)
}
