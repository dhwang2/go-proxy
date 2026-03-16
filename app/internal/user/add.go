package user

import (
	"fmt"

	"go-proxy/internal/crypto"
	"go-proxy/internal/store"
)

// Add creates a new user and adds them to all existing inbounds.
func Add(s *store.Store, name string) error {
	if name == "" {
		return fmt.Errorf("user name cannot be empty")
	}

	// Check for duplicate name across inbounds.
	for _, ib := range s.SingBox.Inbounds {
		for _, u := range ib.Users {
			if u.Name == name {
				return fmt.Errorf("user %q already exists in inbound %s", name, ib.Tag)
			}
		}
	}

	for i := range s.SingBox.Inbounds {
		ib := &s.SingBox.Inbounds[i]
		user, err := newUserForInbound(ib, name)
		if err != nil {
			return fmt.Errorf("create user for %s: %w", ib.Tag, err)
		}
		if user != nil {
			ib.Users = append(ib.Users, *user)

			// Record in user-management.json.
			key := store.UserKey(ib.Type, ib.Tag, user.Credential())
			s.UserMeta.Name[key] = name
		}
	}

	// Register in default group.
	const defaultGroup = "~/.groups"
	if s.UserMeta.Groups == nil {
		s.UserMeta.Groups = make(map[string][]string)
	}
	members := s.UserMeta.Groups[defaultGroup]
	for _, m := range members {
		if m == name {
			goto skipGroup
		}
	}
	s.UserMeta.Groups[defaultGroup] = append(members, name)
skipGroup:

	s.MarkDirty(store.FileSingBox)
	s.MarkDirty(store.FileUserMeta)
	return nil
}

// newUserForInbound creates an appropriate user object for the given inbound type.
func newUserForInbound(ib *store.Inbound, name string) (*store.User, error) {
	switch ib.Type {
	case "vless":
		uuid, err := crypto.GenerateUUID()
		if err != nil {
			return nil, err
		}
		flow := ""
		// Inherit flow from existing users if present.
		if len(ib.Users) > 0 {
			flow = ib.Users[0].Flow
		}
		return &store.User{Name: name, UUID: uuid, Flow: flow}, nil

	case "tuic":
		uuid, err := crypto.GenerateUUID()
		if err != nil {
			return nil, err
		}
		pw, err := crypto.GeneratePassword(16)
		if err != nil {
			return nil, err
		}
		return &store.User{Name: name, UUID: uuid, Password: pw}, nil

	case "trojan", "anytls":
		pw, err := crypto.GeneratePassword(16)
		if err != nil {
			return nil, err
		}
		return &store.User{Name: name, Password: pw}, nil

	case "shadowsocks":
		method := ib.Method
		if method == "" {
			method = crypto.DefaultSSMethod
		}
		keySize := crypto.SSKeySize(method)
		key, err := crypto.GenerateSSKey(keySize)
		if err != nil {
			return nil, err
		}
		return &store.User{Name: name, Password: key}, nil

	default:
		// Unknown inbound type; skip user creation.
		return nil, nil
	}
}
