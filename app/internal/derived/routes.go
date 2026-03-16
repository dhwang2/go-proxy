package derived

import "go-proxy/internal/store"

// UserRoutes returns the routing rules assigned to a specific user.
func UserRoutes(s *store.Store, userName string) []store.UserRouteRule {
	var result []store.UserRouteRule
	for _, r := range s.UserRoutes {
		for _, u := range r.AuthUser {
			if u == userName {
				result = append(result, r)
				break
			}
		}
	}
	return result
}

// AllRoutedUsers returns all unique user names that have routing rules.
func AllRoutedUsers(s *store.Store) []string {
	seen := make(map[string]bool)
	var users []string
	for _, r := range s.UserRoutes {
		for _, u := range r.AuthUser {
			if !seen[u] {
				seen[u] = true
				users = append(users, u)
			}
		}
	}
	return users
}

// RouteRuleCount returns the number of routing rules per user.
func RouteRuleCount(s *store.Store) map[string]int {
	counts := make(map[string]int)
	for _, r := range s.UserRoutes {
		for _, u := range r.AuthUser {
			counts[u]++
		}
	}
	return counts
}
