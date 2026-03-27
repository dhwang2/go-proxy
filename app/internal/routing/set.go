package routing

import (
	"fmt"

	"go-proxy/internal/store"
)

// SetRule adds or updates a routing rule for a user.
func SetRule(s *store.Store, userName string, rule store.UserRouteRule) error {
	if userName == "" {
		return fmt.Errorf("user name cannot be empty")
	}
	if rule.Outbound == "" {
		return fmt.Errorf("outbound cannot be empty")
	}

	// Ensure action is set.
	if rule.Action == "" {
		rule.Action = "route"
	}

	// Add the user to auth_user if not already present.
	hasUser := false
	for _, u := range rule.AuthUser {
		if u == userName {
			hasUser = true
			break
		}
	}
	if !hasUser {
		rule.AuthUser = append(rule.AuthUser, userName)
	}

	for i := range s.UserRoutes {
		if !hasAuthUser(s.UserRoutes[i].AuthUser, userName) {
			continue
		}
		if !rulesMatchForDedup(s.UserRoutes[i], rule) {
			continue
		}
		{
			if len(s.UserRoutes[i].AuthUser) > 1 {
				s.UserRoutes[i].AuthUser = removeAuthUser(s.UserRoutes[i].AuthUser, userName)
				rule.AuthUser = []string{userName}
				s.UserRoutes = append(s.UserRoutes[:i+1], append([]store.UserRouteRule{rule}, s.UserRoutes[i+1:]...)...)
				s.MarkDirty(store.FileUserRoutes)
				return nil
			}
			s.UserRoutes[i] = rule
			s.MarkDirty(store.FileUserRoutes)
			return nil
		}
	}

	s.UserRoutes = append(s.UserRoutes, rule)
	s.MarkDirty(store.FileUserRoutes)
	return nil
}

// rulesMatchForDedup checks whether two rules target the same routing concern.
// Uses preset-aware matching: if both rules resolve to the same preset, they match
// even if stored fields differ (e.g., old rule lacks FallbackDomains).
func rulesMatchForDedup(existing, incoming store.UserRouteRule) bool {
	if ep, ok := presetForRule(existing); ok {
		if ip, ok2 := presetForRule(incoming); ok2 {
			return ep.Name == ip.Name
		}
	}
	return sameStringSlice(existing.RuleSet, incoming.RuleSet) &&
		sameStringSlice(existing.Domain, incoming.Domain) &&
		sameStringSlice(existing.DomainSuffix, incoming.DomainSuffix) &&
		sameStringSlice(existing.DomainKeyword, incoming.DomainKeyword) &&
		sameStringSlice(existing.DomainRegex, incoming.DomainRegex) &&
		sameStringSlice(existing.IPCIDR, incoming.IPCIDR)
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func removeAuthUser(users []string, userName string) []string {
	var kept []string
	for _, name := range users {
		if name != userName {
			kept = append(kept, name)
		}
	}
	return kept
}
