package derived

import "go-proxy/internal/store"

// DashboardStats summarizes the current proxy state.
type DashboardStats struct {
	ProtocolCount int
	UserCount     int
	RouteCount    int
	TemplateCount int
}

// Dashboard computes dashboard statistics from the store.
func Dashboard(s *store.Store) DashboardStats {
	users := make(map[string]bool)
	for _, ib := range s.SingBox.Inbounds {
		for _, u := range ib.Users {
			users[u.Name] = true
		}
	}

	return DashboardStats{
		ProtocolCount: len(s.SingBox.Inbounds),
		UserCount:     len(users),
		RouteCount:    len(s.UserRoutes),
		TemplateCount: len(s.UserTemplate.Templates),
	}
}
