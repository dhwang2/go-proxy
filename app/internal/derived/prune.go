package derived

import "go-proxy/internal/store"

// PruneOrphanAuthUsers removes auth_user entries from route and DNS rules
// for users that no longer exist.
func PruneOrphanAuthUsers(s *store.Store, activeUsers map[string]bool) bool {
	changed := false

	var kept []store.UserRouteRule
	for _, r := range s.UserRoutes {
		origLen := len(r.AuthUser)
		var validUsers []string
		for _, u := range r.AuthUser {
			if activeUsers[u] {
				validUsers = append(validUsers, u)
			}
		}
		if len(validUsers) > 0 {
			if len(validUsers) != origLen {
				changed = true
			}
			r.AuthUser = validUsers
			kept = append(kept, r)
		} else if origLen > 0 {
			changed = true
		}
	}
	s.UserRoutes = kept

	if s.SingBox.Route != nil {
		var keptRoute []store.RouteRule
		for _, r := range s.SingBox.Route.Rules {
			if len(r.AuthUser) == 0 {
				keptRoute = append(keptRoute, r)
				continue
			}
			var validUsers []string
			for _, u := range r.AuthUser {
				if activeUsers[u] {
					validUsers = append(validUsers, u)
				}
			}
			if len(validUsers) > 0 {
				r.AuthUser = validUsers
				keptRoute = append(keptRoute, r)
			} else {
				changed = true
			}
		}
		s.SingBox.Route.Rules = keptRoute
	}

	if s.SingBox.DNS != nil {
		var keptDNS []store.DNSRule
		for _, r := range s.SingBox.DNS.Rules {
			if len(r.AuthUser) == 0 {
				keptDNS = append(keptDNS, r)
				continue
			}
			var validUsers []string
			for _, u := range r.AuthUser {
				if activeUsers[u] {
					validUsers = append(validUsers, u)
				}
			}
			if len(validUsers) > 0 {
				r.AuthUser = validUsers
				keptDNS = append(keptDNS, r)
			} else {
				changed = true
			}
		}
		s.SingBox.DNS.Rules = keptDNS
	}

	return changed
}
