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
	Template    string
	Expiry      string
}

// List returns information about all users.
func List(s *store.Store) []Info {
	membership := derived.Membership(s)
	routeCounts := derived.RouteRuleCount(s)

	// Build template and expiry lookups by display name.
	templateByName := make(map[string]string)
	expiryByName := make(map[string]string)
	for key, tmpl := range s.UserMeta.Template {
		if name, ok := s.UserMeta.Name[key]; ok {
			templateByName[name] = tmpl
		}
	}
	for key, exp := range s.UserMeta.Expiry {
		if name, ok := s.UserMeta.Name[key]; ok {
			expiryByName[name] = exp
		}
	}

	names := derived.UserNames(s)
	result := make([]Info, 0, len(names))
	for _, name := range names {
		info := Info{
			Name:        name,
			Memberships: membership[name],
			RouteCount:  routeCounts[name],
			Template:    templateByName[name],
			Expiry:      expiryByName[name],
		}
		result = append(result, info)
	}
	return result
}
