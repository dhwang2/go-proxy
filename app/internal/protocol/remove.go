package protocol

import (
	"fmt"

	"go-proxy/internal/store"
)

// Remove removes a protocol inbound from the store by tag.
func Remove(s *store.Store, tag string) error {
	if tag == store.SnellTag {
		if s.SnellConf == nil {
			return fmt.Errorf("inbound %q not found", tag)
		}
		s.SnellConf = nil
		s.MarkDirty(store.FileSnellConf)
		cleanupUserMeta(s, tag)
		return nil
	}

	idx := -1
	for i, ib := range s.SingBox.Inbounds {
		if ib.Tag == tag {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("inbound %q not found", tag)
	}

	// Remove the inbound.
	s.SingBox.Inbounds = append(s.SingBox.Inbounds[:idx], s.SingBox.Inbounds[idx+1:]...)
	s.MarkDirty(store.FileSingBox)

	// Clean up user metadata references for this inbound.
	cleanupUserMeta(s, tag)

	return nil
}

// cleanupUserMeta removes all user-management.json entries referencing the given tag.
func cleanupUserMeta(s *store.Store, tag string) {
	changed := false
	for key, entry := range s.UserMeta.Disabled {
		if entry.Tag == tag {
			delete(s.UserMeta.Disabled, key)
			changed = true
		}
	}
	// Remove entries whose keys contain the tag.
	for key := range s.UserMeta.Expiry {
		if keyHasTag(key, tag) {
			delete(s.UserMeta.Expiry, key)
			changed = true
		}
	}
	for key := range s.UserMeta.Route {
		if keyHasTag(key, tag) {
			delete(s.UserMeta.Route, key)
			changed = true
		}
	}
	for key := range s.UserMeta.Template {
		if keyHasTag(key, tag) {
			delete(s.UserMeta.Template, key)
			changed = true
		}
	}
	for key := range s.UserMeta.Name {
		if keyHasTag(key, tag) {
			delete(s.UserMeta.Name, key)
			changed = true
		}
	}
	if changed {
		s.MarkDirty(store.FileUserMeta)
	}
}

// keyHasTag checks if a user key (proto|tag|id) has the given tag segment.
func keyHasTag(key, tag string) bool {
	_, t, _ := store.ParseUserKey(key)
	return t == tag
}

// RemoveUserFromInbound removes a single user from an inbound.
// If the user is the last one, the entire inbound is removed.
func RemoveUserFromInbound(s *store.Store, tag, userName string) error {
	if tag == store.SnellTag {
		return Remove(s, tag)
	}

	idx := -1
	for i, ib := range s.SingBox.Inbounds {
		if ib.Tag == tag {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("inbound %q not found", tag)
	}

	ib := &s.SingBox.Inbounds[idx]

	// Shadowsocks single-user: remove entire inbound.
	if ib.Type == "shadowsocks" && len(ib.Users) == 0 {
		return Remove(s, tag)
	}

	// Find and remove the user.
	userIdx := -1
	for i, u := range ib.Users {
		if u.Name == userName {
			userIdx = i
			break
		}
	}
	if userIdx == -1 {
		return fmt.Errorf("user %q not found in inbound %q", userName, tag)
	}

	// If last user, remove entire inbound.
	if len(ib.Users) == 1 {
		return Remove(s, tag)
	}

	// Remove just this user.
	ib.Users = append(ib.Users[:userIdx], ib.Users[userIdx+1:]...)
	s.MarkDirty(store.FileSingBox)

	cleanupUserMetaForUser(s, tag, userName)
	return nil
}

// cleanupUserMetaForUser removes all user-meta entries for a specific user+tag combination.
func cleanupUserMetaForUser(s *store.Store, tag, userName string) {
	// Find the meta key for this user+tag combination.
	var targetKey string
	for key, name := range s.UserMeta.Name {
		_, t, _ := store.ParseUserKey(key)
		if t == tag && name == userName {
			targetKey = key
			break
		}
	}
	if targetKey == "" {
		return
	}

	changed := false
	if _, ok := s.UserMeta.Name[targetKey]; ok {
		delete(s.UserMeta.Name, targetKey)
		changed = true
	}
	if _, ok := s.UserMeta.Expiry[targetKey]; ok {
		delete(s.UserMeta.Expiry, targetKey)
		changed = true
	}
	if _, ok := s.UserMeta.Route[targetKey]; ok {
		delete(s.UserMeta.Route, targetKey)
		changed = true
	}
	if _, ok := s.UserMeta.Template[targetKey]; ok {
		delete(s.UserMeta.Template, targetKey)
		changed = true
	}
	if _, ok := s.UserMeta.Disabled[targetKey]; ok {
		delete(s.UserMeta.Disabled, targetKey)
		changed = true
	}
	if changed {
		s.MarkDirty(store.FileUserMeta)
	}
}
