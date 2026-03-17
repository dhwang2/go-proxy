package derived

import (
	"fmt"
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
	Ports         string // comma-separated port numbers or "none"
}

// Dashboard computes dashboard statistics from the store.
func Dashboard(s *store.Store) DashboardStats {
	var protocols []string
	var ports []string

	for _, ib := range s.SingBox.Inbounds {
		if ib.Type != "" {
			protocols = append(protocols, ib.Type)
		}
		if ib.ListenPort > 0 {
			ports = append(ports, fmt.Sprintf("%d", ib.ListenPort))
		}
	}

	protoStr := "none"
	if len(protocols) > 0 {
		protoStr = strings.Join(protocols, ", ")
	}

	portStr := "none"
	if len(ports) > 0 {
		portStr = strings.Join(ports, ", ")
	}

	return DashboardStats{
		ProtocolCount: len(s.SingBox.Inbounds),
		UserCount:     len(UserNames(s)),
		RouteCount:    len(s.UserRoutes),
		TemplateCount: len(s.UserTemplate.Templates),
		Protocols:     protoStr,
		Ports:         portStr,
	}
}
