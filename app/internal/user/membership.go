package user

import (
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// MembershipSummary returns a formatted summary of user protocol memberships.
func MembershipSummary(s *store.Store) map[string][]string {
	m := derived.Membership(s)
	result := make(map[string][]string)
	for name, entries := range m {
		var protocols []string
		for _, e := range entries {
			protocols = append(protocols, e.Tag)
		}
		result[name] = protocols
	}
	return result
}
