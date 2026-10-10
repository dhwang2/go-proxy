package routing

import (
	"slices"

	"go-proxy/internal/store"
)

// A stored rule is one user's preset and the outbound it sends that traffic
// to. Every change below selects a user's rules by preset name.

func userRuleFor(rule store.UserRouteRule, userName string, names []string) bool {
	if !slices.Contains(rule.AuthUser, userName) {
		return false
	}
	preset, ok := PresetOf(rule)
	return ok && slices.Contains(names, preset.Name)
}

// AddUserPreset adds the user's rule for preset, sending it to outbound. A
// preset the user already has a rule for is left as it is, whatever outbound
// it uses: changing that is SetUserPresetsOutbound's. It reports whether a
// rule was added.
func AddUserPreset(s *store.Store, userName string, preset Preset, outbound string) bool {
	if slices.ContainsFunc(s.UserRoutes, func(rule store.UserRouteRule) bool {
		return userRuleFor(rule, userName, []string{preset.Name})
	}) {
		return false
	}
	s.UserRoutes = append(s.UserRoutes, PresetToRule(preset, userName, outbound))
	s.MarkDirty(store.FileUserRoutes)
	return true
}

// SetUserPresetsOutbound points the user's rules for the named presets at
// outbound and returns how many changed.
func SetUserPresetsOutbound(s *store.Store, userName string, names []string, outbound string) int {
	changed := 0
	for i := range s.UserRoutes {
		if userRuleFor(s.UserRoutes[i], userName, names) && s.UserRoutes[i].Outbound != outbound {
			s.UserRoutes[i].Outbound = outbound
			changed++
		}
	}
	if changed > 0 {
		s.MarkDirty(store.FileUserRoutes)
	}
	return changed
}

// RemoveUserPresets removes the user's rules for the named presets and
// returns how many it removed.
func RemoveUserPresets(s *store.Store, userName string, names []string) int {
	return removeRules(s, func(rule store.UserRouteRule) bool { return userRuleFor(rule, userName, names) })
}

// ClearUser removes every rule of one user.
func ClearUser(s *store.Store, userName string) int {
	return removeRules(s, func(rule store.UserRouteRule) bool { return slices.Contains(rule.AuthUser, userName) })
}

// ClearAll removes every user's rules.
func ClearAll(s *store.Store) int {
	return removeRules(s, func(store.UserRouteRule) bool { return true })
}

func removeRules(s *store.Store, match func(store.UserRouteRule) bool) int {
	before := len(s.UserRoutes)
	s.UserRoutes = slices.DeleteFunc(s.UserRoutes, match)
	removed := before - len(s.UserRoutes)
	if removed > 0 {
		s.MarkDirty(store.FileUserRoutes)
	}
	return removed
}
