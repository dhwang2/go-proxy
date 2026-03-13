package routing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dhwang2/go-proxy/internal/derived"
	"github.com/dhwang2/go-proxy/internal/store"
)

type RouteRow struct {
	Outbound string
	AuthUser []string
	RuleSet  []string
}

type MutationResult struct {
	ConfigChanged bool
	RouteChanged  int
	DNSChanged    int
}

func (r MutationResult) Changed() bool {
	return r.ConfigChanged
}

func ListRules(st *store.Store, user string) []RouteRow {
	if st == nil || st.Config == nil {
		return nil
	}
	target := normalizeName(user)
	rows := make([]RouteRow, 0, len(st.Config.Route.Rules))
	for _, rule := range st.Config.Route.Rules {
		if strings.TrimSpace(rule.Action) != "route" || len(rule.RuleSet) == 0 {
			continue
		}
		if target != "" && !containsUser(rule.AuthUser, target) {
			continue
		}
		rows = append(rows, RouteRow{
			Outbound: strings.TrimSpace(rule.Outbound),
			AuthUser: append([]string(nil), rule.AuthUser...),
			RuleSet:  append([]string(nil), rule.RuleSet...),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Outbound != rows[j].Outbound {
			return rows[i].Outbound < rows[j].Outbound
		}
		return strings.Join(rows[i].AuthUser, ",") < strings.Join(rows[j].AuthUser, ",")
	})
	return rows
}

func UpsertUserRule(st *store.Store, user, outbound string, ruleSets []string) (MutationResult, error) {
	if st == nil || st.Config == nil || st.UserMeta == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	user = normalizeName(user)
	outbound = strings.TrimSpace(outbound)
	ruleSets = normalizeRuleSets(ruleSets)
	if user == "" || outbound == "" || len(ruleSets) == 0 {
		return MutationResult{}, fmt.Errorf("usage: routing set <user> <outbound> <rule_set_csv>")
	}
	if _, ok := st.UserMeta.Groups[user]; !ok {
		return MutationResult{}, fmt.Errorf("group not found: %s", user)
	}
	if !outboundExists(st.Config, outbound) {
		return MutationResult{}, fmt.Errorf("outbound not found: %s", outbound)
	}

	next := make([]store.RouteRule, 0, len(st.Config.Route.Rules)+1)
	removed := 0
	for _, rule := range st.Config.Route.Rules {
		if strings.TrimSpace(rule.Action) != "route" || len(rule.RuleSet) == 0 {
			next = append(next, rule)
			continue
		}
		filtered := removeUser(rule.AuthUser, user)
		if len(filtered) == 0 && containsUser(rule.AuthUser, user) {
			removed++
			continue
		}
		if len(filtered) != len(rule.AuthUser) {
			rule.AuthUser = filtered
			removed++
		}
		next = append(next, rule)
	}
	next = append(next, store.RouteRule{
		Action:   "route",
		Outbound: outbound,
		AuthUser: []string{user},
		RuleSet:  ruleSets,
	})
	st.Config.Route.Rules = next

	dnsChanged := rebuildManagedDNS(st)
	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		RouteChanged:  removed + 1,
		DNSChanged:    dnsChanged,
	}, nil
}

func ClearUserRule(st *store.Store, user string) (MutationResult, error) {
	if st == nil || st.Config == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	user = normalizeName(user)
	if user == "" {
		return MutationResult{}, fmt.Errorf("usage: routing clear <user>")
	}
	next := make([]store.RouteRule, 0, len(st.Config.Route.Rules))
	removed := 0
	for _, rule := range st.Config.Route.Rules {
		if strings.TrimSpace(rule.Action) != "route" || len(rule.RuleSet) == 0 {
			next = append(next, rule)
			continue
		}
		filtered := removeUser(rule.AuthUser, user)
		if len(rule.AuthUser) > 0 && len(filtered) == 0 && containsUser(rule.AuthUser, user) {
			removed++
			continue
		}
		if len(filtered) != len(rule.AuthUser) {
			rule.AuthUser = filtered
			removed++
		}
		next = append(next, rule)
	}
	if removed == 0 {
		return MutationResult{}, nil
	}
	st.Config.Route.Rules = next
	dnsChanged := rebuildManagedDNS(st)
	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		RouteChanged:  removed,
		DNSChanged:    dnsChanged,
	}, nil
}

func SyncDNS(st *store.Store) (MutationResult, error) {
	if st == nil || st.Config == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	dnsChanged := rebuildManagedDNS(st)
	if dnsChanged == 0 {
		return MutationResult{}, nil
	}
	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		DNSChanged:    dnsChanged,
	}, nil
}

func TestUser(st *store.Store, user string) (map[string]int, int, error) {
	if st == nil || st.Config == nil {
		return nil, 0, fmt.Errorf("store is nil")
	}
	user = normalizeName(user)
	if user == "" {
		return nil, 0, fmt.Errorf("usage: routing test <user>")
	}
	byOutbound := map[string]int{}
	total := 0
	for _, rule := range st.Config.Route.Rules {
		if strings.TrimSpace(rule.Action) != "route" || len(rule.RuleSet) == 0 {
			continue
		}
		if !containsUser(rule.AuthUser, user) {
			continue
		}
		total++
		byOutbound[strings.TrimSpace(rule.Outbound)]++
	}
	return byOutbound, total, nil
}

func EditRules() error {
	return nil
}

func rebuildManagedDNS(st *store.Store) int {
	oldRules := st.Config.DNS.Rules
	managedRoutes := make([]store.RouteRule, 0)
	for _, r := range st.Config.Route.Rules {
		if strings.TrimSpace(r.Action) != "route" || len(r.AuthUser) == 0 || len(r.RuleSet) == 0 {
			continue
		}
		managedRoutes = append(managedRoutes, r)
	}
	unmanagedDNS := make([]store.DNSRule, 0)
	for _, d := range oldRules {
		if strings.TrimSpace(d.Action) == "route" && len(d.AuthUser) > 0 {
			continue
		}
		unmanagedDNS = append(unmanagedDNS, d)
	}
	ctx := derived.DNSContext{
		DefaultServer: firstDNSServer(oldRules),
	}
	if ctx.DefaultServer == "" {
		ctx.DefaultServer = "dns-final"
	}
	managedDNS := derived.DNSRulesFromRoutes(managedRoutes, st.Config.Outbounds, ctx)
	nextDNS := append(unmanagedDNS, managedDNS...)
	changed := 0
	if !sameDNSRules(oldRules, nextDNS) {
		changed = len(nextDNS)
		st.Config.DNS.Rules = nextDNS
	}
	return changed
}

func firstDNSServer(rules []store.DNSRule) string {
	for _, r := range rules {
		if s := strings.TrimSpace(r.Server); s != "" {
			return s
		}
	}
	return ""
}

func sameDNSRules(a, b []store.DNSRule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i].Action) != strings.TrimSpace(b[i].Action) {
			return false
		}
		if strings.TrimSpace(a[i].Server) != strings.TrimSpace(b[i].Server) {
			return false
		}
		if strings.Join(a[i].AuthUser, ",") != strings.Join(b[i].AuthUser, ",") {
			return false
		}
		if strings.Join(a[i].RuleSet, ",") != strings.Join(b[i].RuleSet, ",") {
			return false
		}
	}
	return true
}

func normalizeName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	v = strings.NewReplacer(" ", "-", "\t", "-", "\n", "-").Replace(v)
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	return strings.Trim(v, "-_.")
}

func normalizeRuleSets(v []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(v))
	for _, item := range v {
		for _, seg := range strings.Split(item, ",") {
			s := strings.TrimSpace(seg)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func containsUser(users []string, target string) bool {
	target = normalizeName(target)
	for _, u := range users {
		if normalizeName(u) == target {
			return true
		}
	}
	return false
}

func removeUser(users []string, target string) []string {
	target = normalizeName(target)
	out := make([]string, 0, len(users))
	for _, u := range users {
		if normalizeName(u) == target {
			continue
		}
		out = append(out, u)
	}
	return out
}

func outboundExists(cfg *store.SingboxConfig, outbound string) bool {
	for _, ob := range cfg.Outbounds {
		if strings.TrimSpace(ob.Tag) == outbound {
			return true
		}
	}
	return false
}
