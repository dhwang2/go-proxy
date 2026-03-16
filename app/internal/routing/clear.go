package routing

import "go-proxy/internal/store"

// ClearUser removes all routing rules for a specific user.
func ClearUser(s *store.Store, userName string) int {
	var kept []store.UserRouteRule
	removed := 0

	for _, r := range s.UserRoutes {
		var keptUsers []string
		for _, u := range r.AuthUser {
			if u != userName {
				keptUsers = append(keptUsers, u)
			}
		}
		if len(keptUsers) > 0 {
			r.AuthUser = keptUsers
			kept = append(kept, r)
		} else if len(r.AuthUser) > 0 {
			removed++
		} else {
			// Rules without auth_user are global; keep them.
			kept = append(kept, r)
		}
	}

	if removed > 0 {
		s.UserRoutes = kept
		s.MarkDirty(store.FileUserRoutes)
	}
	return removed
}

// ClearAll removes all user routing rules.
func ClearAll(s *store.Store) int {
	count := len(s.UserRoutes)
	if count > 0 {
		s.UserRoutes = nil
		s.MarkDirty(store.FileUserRoutes)
	}
	return count
}
