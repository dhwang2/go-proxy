package derived

import "go-proxy/internal/store"

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
