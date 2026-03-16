package user

import (
	"fmt"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// Delete removes a user from all inbounds, user-management, and route rules.
func Delete(s *store.Store, name string) error {
	if name == "" {
		return fmt.Errorf("user name cannot be empty")
	}

	found := false

	// Remove from inbounds.
	for i := range s.SingBox.Inbounds {
		ib := &s.SingBox.Inbounds[i]
		var kept []store.User
		for _, u := range ib.Users {
			if u.Name == name {
				found = true
				// Remove corresponding metadata.
				key := store.UserKey(ib.Type, ib.Tag, u.Credential())
				delete(s.UserMeta.Disabled, key)
				delete(s.UserMeta.Expiry, key)
				delete(s.UserMeta.Route, key)
				delete(s.UserMeta.Template, key)
				delete(s.UserMeta.Name, key)
			} else {
				kept = append(kept, u)
			}
		}
		ib.Users = kept
	}

	if !found {
		return fmt.Errorf("user %q not found", name)
	}

	// Remove from groups.
	for groupName, members := range s.UserMeta.Groups {
		var kept []string
		for _, m := range members {
			if m != name {
				kept = append(kept, m)
			}
		}
		if len(kept) == 0 {
			delete(s.UserMeta.Groups, groupName)
		} else {
			s.UserMeta.Groups[groupName] = kept
		}
	}

	// Prune orphan auth_user references from route and DNS rules.
	activeUsers := make(map[string]bool)
	for _, n := range derived.UserNames(s) {
		activeUsers[n] = true
	}
	if derived.PruneOrphanAuthUsers(s, activeUsers) {
		s.MarkDirty(store.FileUserRoutes)
	}

	s.MarkDirty(store.FileSingBox)
	s.MarkDirty(store.FileUserMeta)
	return nil
}
