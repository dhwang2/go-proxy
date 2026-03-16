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

	s.UserRoutes = append(s.UserRoutes, rule)
	s.MarkDirty(store.FileUserRoutes)
	return nil
}
