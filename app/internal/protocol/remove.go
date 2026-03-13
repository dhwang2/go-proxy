package protocol

import (
	"fmt"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

func Remove(st *store.Store, target string) (MutationResult, error) {
	if st == nil || st.Config == nil || st.UserMeta == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return MutationResult{}, fmt.Errorf("usage: proxy protocol remove <protocol|tag>")
	}
	if proto, err := normalizeProtocol(target); err == nil {
		return removeByProtocol(st, proto), nil
	}
	return removeByTag(st, target), nil
}

func removeByProtocol(st *store.Store, proto string) MutationResult {
	result := MutationResult{}
	if proto == "snell" {
		if st.SnellConf != nil {
			psk := strings.TrimSpace(st.SnellConf.Get("psk"))
			if len(st.SnellConf.Values) > 0 {
				st.SnellConf.Values = map[string]string{}
				result.SnellChanged = true
			}
			if psk != "" {
				if dropMetaForUser(st.UserMeta, "snell", "snell-v5", psk) {
					result.MetaChanged = true
					result.UpdatedMetaRows++
				}
			}
		}
		if result.SnellChanged {
			st.MarkSnellDirty()
		}
		if result.MetaChanged {
			st.MarkUserMetaDirty()
		}
		return result
	}

	next := make([]store.Inbound, 0, len(st.Config.Inbounds))
	for _, in := range st.Config.Inbounds {
		if normalizeProtocolType(in.Type) != proto {
			next = append(next, in)
			continue
		}
		result.RemovedInbounds++
		for _, u := range in.Users {
			if dropMetaForUser(st.UserMeta, proto, in.Tag, u.Key()) {
				result.MetaChanged = true
				result.UpdatedMetaRows++
			}
		}
	}
	if result.RemovedInbounds > 0 {
		st.Config.Inbounds = next
		result.ConfigChanged = true
		st.MarkConfigDirty()
	}
	if result.MetaChanged {
		st.MarkUserMetaDirty()
	}
	return result
}

func removeByTag(st *store.Store, tag string) MutationResult {
	result := MutationResult{}
	tag = strings.TrimSpace(tag)
	next := make([]store.Inbound, 0, len(st.Config.Inbounds))
	for _, in := range st.Config.Inbounds {
		if strings.TrimSpace(in.Tag) != tag {
			next = append(next, in)
			continue
		}
		result.RemovedInbounds++
		proto := normalizeProtocolType(in.Type)
		for _, u := range in.Users {
			if dropMetaForUser(st.UserMeta, proto, in.Tag, u.Key()) {
				result.MetaChanged = true
				result.UpdatedMetaRows++
			}
		}
	}
	if result.RemovedInbounds > 0 {
		st.Config.Inbounds = next
		result.ConfigChanged = true
		st.MarkConfigDirty()
	}
	if result.MetaChanged {
		st.MarkUserMetaDirty()
	}
	return result
}

func dropMetaForUser(meta *store.UserMeta, proto, tag, userID string) bool {
	if meta == nil {
		return false
	}
	meta.EnsureDefaults()
	key := userMetaKey(proto, tag, userID)
	changed := false
	if _, ok := meta.Name[key]; ok {
		delete(meta.Name, key)
		changed = true
	}
	if _, ok := meta.Template[key]; ok {
		delete(meta.Template, key)
		changed = true
	}
	if _, ok := meta.Route[key]; ok {
		delete(meta.Route, key)
		changed = true
	}
	if _, ok := meta.Expiry[key]; ok {
		delete(meta.Expiry, key)
		changed = true
	}
	if _, ok := meta.Disabled[key]; ok {
		delete(meta.Disabled, key)
		changed = true
	}
	return changed
}
