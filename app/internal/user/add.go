package user

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/store"
)

func ValidateName(name string) error {
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("user name must contain 1 to 64 characters")
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsNumber(c) && c != '_' && c != '-' && c != '.' {
			return fmt.Errorf("user name must contain only letters, numbers, dots, underscores or hyphens")
		}
	}
	return nil
}

func Exists(s *store.Store, name string) bool {
	for _, current := range derived.UserNames(s) {
		if current == name {
			return true
		}
	}
	return false
}

func Add(s *store.Store, name string, allProtocols bool) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}
	changed := false
	if !Exists(s, name) {
		s.UserMeta.Groups["default"] = append(s.UserMeta.Groups["default"], name)
		s.MarkDirty(store.FileUserMeta)
		changed = true
	}
	if allProtocols {
		for i := range s.SingBox.Inbounds {
			ib := &s.SingBox.Inbounds[i]
			if ib.FindUser(name) != nil {
				continue
			}
			if _, err := protocol.AddUserToExisting(s, ib, name); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}
