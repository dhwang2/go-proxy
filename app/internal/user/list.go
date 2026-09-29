package user

import (
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// Info holds aggregated information about a user.
type Info struct {
	Name        string
	Memberships []derived.MembershipEntry
	RouteCount  int
}

// List returns information about all users.
func List(s *store.Store) []Info {
	membership := derived.Membership(s)
	routeCounts := derived.RouteRuleCount(s)

	names := derived.UserNames(s)
	result := make([]Info, 0, len(names))
	for _, name := range names {
		info := Info{
			Name:        name,
			Memberships: membership[name],
			RouteCount:  routeCounts[name],
		}
		result = append(result, info)
	}
	return result
}
