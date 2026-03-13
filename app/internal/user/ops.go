package user

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dhwang2/go-proxy/internal/derived"
	"github.com/dhwang2/go-proxy/internal/store"
)

type GroupStats struct {
	Name      string
	Active    int
	Disabled  int
	Protocols int
}

type MutationResult struct {
	MetaChanged       bool
	ConfigChanged     bool
	AffectedUsers     int
	AffectedRouteRows int
	AffectedDNSRows   int
}

func (r MutationResult) Changed() bool {
	return r.MetaChanged || r.ConfigChanged
}

func ListGroups(st *store.Store) []GroupStats {
	if st == nil || st.UserMeta == nil {
		return nil
	}
	stats := map[string]*GroupStats{}
	protocols := map[string]map[string]struct{}{}

	for name := range st.UserMeta.Groups {
		n := NormalizeGroupName(name)
		if n == "" {
			continue
		}
		stats[n] = &GroupStats{Name: n}
		protocols[n] = map[string]struct{}{}
	}

	for _, row := range derived.ComputeMemberships(st.Config, st.UserMeta) {
		group := NormalizeGroupName(row.UserName)
		if group == "" {
			continue
		}
		item, ok := stats[group]
		if !ok {
			item = &GroupStats{Name: group}
			stats[group] = item
		}
		if protocols[group] == nil {
			protocols[group] = map[string]struct{}{}
		}
		protocols[group][strings.TrimSpace(row.Protocol)] = struct{}{}
		if row.State == "disabled" {
			item.Disabled++
		} else {
			item.Active++
		}
	}

	out := make([]GroupStats, 0, len(stats))
	for name, item := range stats {
		item.Protocols = len(protocols[name])
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func AddGroup(st *store.Store, name string) (MutationResult, error) {
	if st == nil || st.UserMeta == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	name = NormalizeGroupName(name)
	if name == "" {
		return MutationResult{}, fmt.Errorf("invalid group name")
	}
	if _, exists := st.UserMeta.Groups[name]; exists {
		return MutationResult{}, nil
	}
	st.UserMeta.AddGroup(name)
	st.MarkUserMetaDirty()
	return MutationResult{MetaChanged: true}, nil
}

func RenameGroup(st *store.Store, oldName, newName string) (MutationResult, error) {
	if st == nil || st.UserMeta == nil || st.Config == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	oldName = NormalizeGroupName(oldName)
	newName = NormalizeGroupName(newName)
	if oldName == "" || newName == "" {
		return MutationResult{}, fmt.Errorf("invalid group name")
	}
	if oldName == newName {
		return MutationResult{}, nil
	}
	if _, exists := st.UserMeta.Groups[newName]; exists {
		return MutationResult{}, fmt.Errorf("target group already exists: %s", newName)
	}

	result := MutationResult{}
	if _, exists := st.UserMeta.Groups[oldName]; exists {
		st.UserMeta.RenameGroup(oldName, newName)
		result.MetaChanged = true
	}

	for key, value := range st.UserMeta.Name {
		if NormalizeGroupName(value) != oldName {
			continue
		}
		st.UserMeta.Name[key] = newName
		result.MetaChanged = true
		result.AffectedUsers++
	}

	for key, disabled := range st.UserMeta.Disabled {
		if NormalizeGroupName(disabled.User.Name) != oldName {
			continue
		}
		disabled.User.Name = newName
		st.UserMeta.Disabled[key] = disabled
		result.MetaChanged = true
		result.AffectedUsers++
	}

	if renameUsersInConfig(st.Config, oldName, newName) {
		result.ConfigChanged = true
	}
	routeRules, routeChanged, routeAffected := renameAuthUser(st.Config.Route.Rules, oldName, newName)
	if routeChanged {
		st.Config.Route.Rules = routeRules
		result.ConfigChanged = true
		result.AffectedRouteRows += routeAffected
	}
	dnsRules, dnsChanged, dnsAffected := renameDNSAuthUser(st.Config.DNS.Rules, oldName, newName)
	if dnsChanged {
		st.Config.DNS.Rules = dnsRules
		result.ConfigChanged = true
		result.AffectedDNSRows += dnsAffected
	}

	if result.MetaChanged {
		st.MarkUserMetaDirty()
	}
	if result.ConfigChanged {
		st.MarkConfigDirty()
	}
	return result, nil
}

func DeleteGroup(st *store.Store, name string) (MutationResult, error) {
	if st == nil || st.UserMeta == nil || st.Config == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	name = NormalizeGroupName(name)
	if name == "" {
		return MutationResult{}, fmt.Errorf("invalid group name")
	}

	result := MutationResult{}
	removeKeys := map[string]struct{}{}
	for key, value := range st.UserMeta.Name {
		if NormalizeGroupName(value) != name {
			continue
		}
		removeKeys[key] = struct{}{}
		delete(st.UserMeta.Name, key)
		delete(st.UserMeta.Template, key)
		delete(st.UserMeta.Expiry, key)
		delete(st.UserMeta.Route, key)
		delete(st.UserMeta.Disabled, key)
		result.MetaChanged = true
		result.AffectedUsers++
	}
	for key, disabled := range st.UserMeta.Disabled {
		if NormalizeGroupName(disabled.User.Name) != name {
			continue
		}
		delete(st.UserMeta.Disabled, key)
		delete(st.UserMeta.Template, key)
		delete(st.UserMeta.Expiry, key)
		delete(st.UserMeta.Route, key)
		delete(st.UserMeta.Name, key)
		result.MetaChanged = true
		result.AffectedUsers++
	}
	if _, exists := st.UserMeta.Groups[name]; exists {
		st.UserMeta.DeleteGroup(name)
		result.MetaChanged = true
	}

	if removeUsersFromConfig(st.Config, name, removeKeys) {
		result.ConfigChanged = true
	}
	routeRules, routeChanged, routeAffected := removeAuthUser(st.Config.Route.Rules, name)
	if routeChanged {
		st.Config.Route.Rules = routeRules
		result.ConfigChanged = true
		result.AffectedRouteRows += routeAffected
	}
	dnsRules, dnsChanged, dnsAffected := removeDNSAuthUser(st.Config.DNS.Rules, name)
	if dnsChanged {
		st.Config.DNS.Rules = dnsRules
		result.ConfigChanged = true
		result.AffectedDNSRows += dnsAffected
	}

	if result.MetaChanged {
		st.MarkUserMetaDirty()
	}
	if result.ConfigChanged {
		st.MarkConfigDirty()
	}
	return result, nil
}

func NormalizeGroupName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "\t", "-", "\n", "-")
	v = replacer.Replace(v)
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	v = strings.Trim(v, "-_.")
	if v == "" {
		return "user"
	}
	return v
}

func renameUsersInConfig(cfg *store.SingboxConfig, oldName, newName string) bool {
	if cfg == nil {
		return false
	}
	changed := false
	for i := range cfg.Inbounds {
		for j := range cfg.Inbounds[i].Users {
			current := NormalizeGroupName(cfg.Inbounds[i].Users[j].Name)
			if current != oldName {
				continue
			}
			cfg.Inbounds[i].Users[j].Name = newName
			changed = true
		}
	}
	return changed
}

func removeUsersFromConfig(cfg *store.SingboxConfig, group string, removeKeys map[string]struct{}) bool {
	if cfg == nil {
		return false
	}
	changed := false
	for i := range cfg.Inbounds {
		in := &cfg.Inbounds[i]
		if len(in.Users) == 0 {
			continue
		}
		filtered := make([]store.User, 0, len(in.Users))
		for _, user := range in.Users {
			id := strings.TrimSpace(user.Key())
			key := userMetaKey(in.Type, in.Tag, id)
			_, removeByKey := removeKeys[key]
			removeByName := NormalizeGroupName(user.Name) == group
			if removeByKey || removeByName {
				changed = true
				continue
			}
			filtered = append(filtered, user)
		}
		if len(filtered) != len(in.Users) {
			in.Users = filtered
		}
	}
	return changed
}

func renameAuthUser(rules []store.RouteRule, oldName, newName string) ([]store.RouteRule, bool, int) {
	if len(rules) == 0 {
		return rules, false, 0
	}
	out := make([]store.RouteRule, 0, len(rules))
	changed := false
	affected := 0
	for _, rule := range rules {
		localChanged := false
		for i, user := range rule.AuthUser {
			if NormalizeGroupName(user) != oldName {
				continue
			}
			rule.AuthUser[i] = newName
			localChanged = true
		}
		if localChanged {
			changed = true
			affected++
		}
		out = append(out, rule)
	}
	return out, changed, affected
}

func renameDNSAuthUser(rules []store.DNSRule, oldName, newName string) ([]store.DNSRule, bool, int) {
	if len(rules) == 0 {
		return rules, false, 0
	}
	out := make([]store.DNSRule, 0, len(rules))
	changed := false
	affected := 0
	for _, rule := range rules {
		localChanged := false
		for i, user := range rule.AuthUser {
			if NormalizeGroupName(user) != oldName {
				continue
			}
			rule.AuthUser[i] = newName
			localChanged = true
		}
		if localChanged {
			changed = true
			affected++
		}
		out = append(out, rule)
	}
	return out, changed, affected
}

func removeAuthUser(rules []store.RouteRule, target string) ([]store.RouteRule, bool, int) {
	if len(rules) == 0 {
		return rules, false, 0
	}
	out := make([]store.RouteRule, 0, len(rules))
	changed := false
	affected := 0
	for _, rule := range rules {
		if len(rule.AuthUser) == 0 {
			out = append(out, rule)
			continue
		}
		filtered := make([]string, 0, len(rule.AuthUser))
		removed := false
		for _, user := range rule.AuthUser {
			if NormalizeGroupName(user) == target {
				removed = true
				continue
			}
			filtered = append(filtered, user)
		}
		if !removed {
			out = append(out, rule)
			continue
		}
		changed = true
		affected++
		if len(filtered) == 0 {
			continue
		}
		rule.AuthUser = filtered
		out = append(out, rule)
	}
	return out, changed, affected
}

func removeDNSAuthUser(rules []store.DNSRule, target string) ([]store.DNSRule, bool, int) {
	if len(rules) == 0 {
		return rules, false, 0
	}
	out := make([]store.DNSRule, 0, len(rules))
	changed := false
	affected := 0
	for _, rule := range rules {
		if len(rule.AuthUser) == 0 {
			out = append(out, rule)
			continue
		}
		filtered := make([]string, 0, len(rule.AuthUser))
		removed := false
		for _, user := range rule.AuthUser {
			if NormalizeGroupName(user) == target {
				removed = true
				continue
			}
			filtered = append(filtered, user)
		}
		if !removed {
			out = append(out, rule)
			continue
		}
		changed = true
		affected++
		if len(filtered) == 0 {
			continue
		}
		rule.AuthUser = filtered
		out = append(out, rule)
	}
	return out, changed, affected
}

func userMetaKey(proto, tag, userID string) string {
	p := strings.ToLower(strings.TrimSpace(proto))
	if p == "shadowsocks" {
		p = "ss"
	}
	return p + "|" + strings.TrimSpace(tag) + "|" + strings.TrimSpace(userID)
}
