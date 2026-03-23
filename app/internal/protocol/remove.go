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
