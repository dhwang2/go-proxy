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
		if sameStringSlice(s.UserRoutes[i].RuleSet, rule.RuleSet) &&
			sameStringSlice(s.UserRoutes[i].Domain, rule.Domain) &&
			sameStringSlice(s.UserRoutes[i].DomainSuffix, rule.DomainSuffix) &&
			sameStringSlice(s.UserRoutes[i].DomainKeyword, rule.DomainKeyword) &&
			sameStringSlice(s.UserRoutes[i].DomainRegex, rule.DomainRegex) &&
			sameStringSlice(s.UserRoutes[i].IPCIDR, rule.IPCIDR) {
			if len(s.UserRoutes[i].AuthUser) > 1 {
				s.UserRoutes[i].AuthUser = removeAuthUser(s.UserRoutes[i].AuthUser, userName)
				rule.AuthUser = []string{userName}
				s.UserRoutes = append(s.UserRoutes[:i+1], append([]store.UserRouteRule{rule}, s.UserRoutes[i+1:]...)...)
				s.MarkDirty(store.FileUserRoutes)
				return nil
			}
			s.UserRoutes[i].Action = rule.Action
			s.UserRoutes[i].Outbound = rule.Outbound
			s.MarkDirty(store.FileUserRoutes)
			return nil
		}
	}

	s.UserRoutes = append(s.UserRoutes, rule)
	s.MarkDirty(store.FileUserRoutes)
	return nil
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
