package derived

import (
	"strings"

	"go-proxy/internal/store"
)

// DashboardStats summarizes the current proxy state.
type DashboardStats struct {
	ProtocolCount int
	UserCount     int
	RouteCount    int
	TemplateCount int
	Protocols     string // comma-separated protocol types or "none"
}

// Dashboard computes dashboard statistics from the store.
func Dashboard(s *store.Store) DashboardStats {
	var protocols []string
	inv := Inventory(s)
	for _, info := range inv {
		if info.Type != "" {
			protocols = append(protocols, info.Type)
		}
	}

	protoStr := "none"
	if len(protocols) > 0 {
		protoStr = strings.Join(protocols, ", ")
	}

	return DashboardStats{
		ProtocolCount: len(inv),
		UserCount:     len(UserNames(s)),
		RouteCount:    len(s.UserRoutes),
		TemplateCount: len(s.UserTemplate.Templates),
		Protocols:     protoStr,
	}
}
