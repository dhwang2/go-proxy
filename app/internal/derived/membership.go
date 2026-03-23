package derived

import (
	"strings"

	"go-proxy/internal/store"
)

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
	if s.SnellConf != nil {
		baseNames := baseUserNames(s)
		if name := snellUserName(s, baseNames); name != "" {
			result[name] = append(result[name], MembershipEntry{
				Proto:    store.SnellTag,
				Tag:      store.SnellTag,
				Port:     s.SnellConf.Port(),
				UserID:   s.SnellConf.PSK,
				UserName: name,
			})
		}
	}
	return result
}

// UserNames returns a deduplicated list of all user names from inbounds and groups.
func UserNames(s *store.Store) []string {
	names := baseUserNames(s)
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	if s.SnellConf != nil {
		if name := snellUserName(s, names); name != "" && !seen[name] {
			names = append(names, name)
		}
	}
	return names
}

func baseUserNames(s *store.Store) []string {
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
	// Include users registered in groups but not yet in any inbound.
	for _, members := range s.UserMeta.Groups {
		for _, name := range members {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func snellUserName(s *store.Store, baseNames []string) string {
	if s.SnellConf == nil || s.SnellConf.PSK == "" {
		return ""
	}
	key := store.UserKey("snell", store.SnellTag, s.SnellConf.PSK)
	if name := strings.TrimSpace(s.UserMeta.Name[key]); name != "" {
		return name
	}
	active := activeInboundUserNames(s)
	if len(active) == 1 {
		return active[0]
	}
	if len(baseNames) == 1 {
		return baseNames[0]
	}
	return ""
}

func activeInboundUserNames(s *store.Store) []string {
	seen := make(map[string]bool)
	var names []string
	for _, ib := range s.SingBox.Inbounds {
		for _, u := range ib.Users {
			if u.Name == "" || seen[u.Name] {
				continue
			}
			seen[u.Name] = true
			names = append(names, u.Name)
		}
	}
	return names
}
