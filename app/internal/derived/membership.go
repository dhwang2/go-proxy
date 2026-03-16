package derived

import "go-proxy/internal/store"

// MembershipEntry describes a user's presence in a protocol inbound.
type MembershipEntry struct {
	Proto    string // inbound type
	Tag      string // inbound tag
	Port     int    // listen port
	UserID   string // uuid or password (the credential)
	UserName string // display name
}

// Membership returns a map of display name → list of protocol memberships.
func Membership(s *store.Store) map[string][]MembershipEntry {
	result := make(map[string][]MembershipEntry)
	for _, ib := range s.SingBox.Inbounds {
		for _, u := range ib.Users {
			entry := MembershipEntry{
				Proto:    ib.Type,
				Tag:      ib.Tag,
				Port:     ib.ListenPort,
				UserID:   u.Credential(),
				UserName: u.Name,
			}
			result[u.Name] = append(result[u.Name], entry)
		}
		// Shadowsocks single-user mode: password at inbound level.
		if ib.Type == "shadowsocks" && len(ib.Users) == 0 && ib.Password != "" {
			entry := MembershipEntry{
				Proto:    ib.Type,
				Tag:      ib.Tag,
				Port:     ib.ListenPort,
				UserID:   ib.Password,
				UserName: "default",
			}
			result["default"] = append(result["default"], entry)
		}
	}
	return result
}

// UserNames returns a deduplicated, sorted list of all user names across inbounds.
func UserNames(s *store.Store) []string {
	seen := make(map[string]bool)
	var names []string
	for _, ib := range s.SingBox.Inbounds {
		for _, u := range ib.Users {
			if !seen[u.Name] {
				seen[u.Name] = true
				names = append(names, u.Name)
			}
		}
	}
	return names
}
