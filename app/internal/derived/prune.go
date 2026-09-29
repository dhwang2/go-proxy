package derived

import "go-proxy/internal/store"

// PruneRouteUsers drops the users that no longer exist from the stored route
// rules, and a rule left with none. The compiled sing-box rules are rebuilt
// from these by routing.Sync.
func PruneRouteUsers(s *store.Store, activeUsers map[string]bool) bool {
	changed := false
	kept := s.UserRoutes[:0]
	for _, r := range s.UserRoutes {
		var valid []string
		for _, u := range r.AuthUser {
			if activeUsers[u] {
				valid = append(valid, u)
			}
		}
		if len(valid) != len(r.AuthUser) {
			changed = true
		}
		if len(valid) > 0 {
			r.AuthUser = valid
			kept = append(kept, r)
		}
	}
	s.UserRoutes = kept
	return changed
}
