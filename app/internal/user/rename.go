package user

import (
	"fmt"

	"go-proxy/internal/store"
)

// Rename changes a user's name across all inbounds, user-management, and route rules.
func Rename(s *store.Store, oldName, newName string) error {
	if oldName == "" || newName == "" {
		return fmt.Errorf("names cannot be empty")
	}
	if oldName == newName {
		return nil
	}

	// Check new name doesn't already exist.
	for _, ib := range s.SingBox.Inbounds {
		for _, u := range ib.Users {
			if u.Name == newName {
				return fmt.Errorf("user %q already exists in inbound %s", newName, ib.Tag)
			}
		}
	}

	found := false

	// Rename in inbounds.
	for i := range s.SingBox.Inbounds {
		for j := range s.SingBox.Inbounds[i].Users {
			if s.SingBox.Inbounds[i].Users[j].Name == oldName {
				s.SingBox.Inbounds[i].Users[j].Name = newName
				found = true
			}
		}
	}

	if !found {
		return fmt.Errorf("user %q not found", oldName)
	}

	// Rename in user-management.json Name map.
	for key, name := range s.UserMeta.Name {
		if name == oldName {
			s.UserMeta.Name[key] = newName
		}
	}

	// Rename in route rules auth_user.
	for i := range s.UserRoutes {
		for j := range s.UserRoutes[i].AuthUser {
			if s.UserRoutes[i].AuthUser[j] == oldName {
				s.UserRoutes[i].AuthUser[j] = newName
			}
		}
	}

	// Rename in sing-box route rules auth_user.
	if s.SingBox.Route != nil {
		for i := range s.SingBox.Route.Rules {
			for j := range s.SingBox.Route.Rules[i].AuthUser {
				if s.SingBox.Route.Rules[i].AuthUser[j] == oldName {
					s.SingBox.Route.Rules[i].AuthUser[j] = newName
				}
			}
		}
	}

	// Rename in sing-box DNS rules auth_user.
	if s.SingBox.DNS != nil {
		for i := range s.SingBox.DNS.Rules {
			for j := range s.SingBox.DNS.Rules[i].AuthUser {
				if s.SingBox.DNS.Rules[i].AuthUser[j] == oldName {
					s.SingBox.DNS.Rules[i].AuthUser[j] = newName
				}
			}
		}
	}

	s.MarkDirty(store.FileSingBox)
	s.MarkDirty(store.FileUserMeta)
	s.MarkDirty(store.FileUserRoutes)
	return nil
}
