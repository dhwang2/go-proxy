package routing

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"go-proxy/internal/store"
)

// authUserKey returns a stable map key for a set of auth_user values.
func authUserKey(users []string) string {
	cp := append([]string(nil), users...)
	sort.Strings(cp)
	return strings.Join(cp, "\x00")
}

// uniqueStrings deduplicates a string slice, preserving first-seen order.
func uniqueStrings(a []string) []string {
	seen := make(map[string]bool, len(a))
	out := make([]string, 0, len(a))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// isGeositeRuleSet returns true if all entries in rs start with "geosite-".
func isGeositeRuleSet(rs []string) bool {
	if len(rs) == 0 {
		return false
	}
	for _, s := range rs {
		if !strings.HasPrefix(s, "geosite-") {
			return false
		}
	}
	return true
}

// isGeoIPRuleSet returns true if all entries in rs start with "geoip-".
func isGeoIPRuleSet(rs []string) bool {
	if len(rs) == 0 {
		return false
	}
	for _, s := range rs {
		if !strings.HasPrefix(s, "geoip-") {
			return false
		}
	}
	return true
}

// isDomainMatchRouteRule returns true if the rule has domain matchers but no rule_set.
func isDomainMatchRouteRule(r store.RouteRule) bool {
	if len(r.RuleSet) > 0 {
		return false
	}
	return len(r.Domain) > 0 || len(r.DomainSuffix) > 0 || len(r.DomainKeyword) > 0 || len(r.DomainRegex) > 0
}

func isPureRuleSetRouteRule(r store.RouteRule) bool {
	return len(r.RuleSet) > 0 &&
		len(r.Domain) == 0 &&
		len(r.DomainSuffix) == 0 &&
		len(r.DomainKeyword) == 0 &&
		len(r.DomainRegex) == 0 &&
		len(r.IPCIDR) == 0
}

// routeRuleClass returns 0 for domain-match, 1 for geosite, 2 for geoip, 3 for other.
func routeRuleClass(r store.RouteRule) int {
	if isDomainMatchRouteRule(r) {
		return 0
	}
	if isGeositeRuleSet(r.RuleSet) {
		return 1
	}
	if isGeoIPRuleSet(r.RuleSet) {
		return 2
	}
	return 3
}

// mergeRouteRulesByOutbound keeps shell-proxy ordering:
// domain-match -> geosite -> geoip -> other.
// Pure geosite/geoip rule_set rules are merged by outbound/action/auth_user.
func mergeRouteRulesByOutbound(rules []store.RouteRule) []store.RouteRule {
	type key struct {
		outbound string
		action   string
		users    string
		class    int
	}

	index := make(map[key]int)
	merged := make([]store.RouteRule, 0, len(rules))

	for _, r := range rules {
		cls := routeRuleClass(r)
		if (cls != 1 && cls != 2) || !isPureRuleSetRouteRule(r) {
			merged = append(merged, r)
			continue
		}
		k := key{
			outbound: r.Outbound,
			action:   r.Action,
			users:    authUserKey(r.AuthUser),
			class:    cls,
		}
		if pos, exists := index[k]; exists {
			m := &merged[pos]
			m.RuleSet = uniqueStrings(append(m.RuleSet, r.RuleSet...))
		} else {
			index[k] = len(merged)
			cp := r
			cp.RuleSet = append([]string(nil), r.RuleSet...)
			merged = append(merged, cp)
		}
	}

	var buckets [4][]store.RouteRule
	for _, r := range merged {
		cls := routeRuleClass(r)
		buckets[cls] = append(buckets[cls], r)
	}
	out := make([]store.RouteRule, 0, len(merged))
	for _, b := range buckets {
		sortRouteRules(b)
		out = append(out, b...)
	}
	return out
}

// sortRouteRules orders one family the way shell-proxy's compiled rules are
// ordered: by outbound, then the users, then the rule sets, and sorts a
// rule-set rule's own tags as shell-proxy does. The order no
// longer depends on when each rule was added, so the same rules always compile
// to the same file. Explicit domain rules stay ahead of every rule-set family,
// as gproxy placed them before; shell-proxy put them after.
func sortRouteRules(rules []store.RouteRule) {
	for index := range rules {
		if isPureRuleSetRouteRule(rules[index]) {
			rules[index].RuleSet = sortedUnique(rules[index].RuleSet)
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		a, b := rules[i], rules[j]
		if a.Outbound != b.Outbound {
			return a.Outbound < b.Outbound
		}
		if users := strings.Join(a.AuthUser, ","); users != strings.Join(b.AuthUser, ",") {
			return users < strings.Join(b.AuthUser, ",")
		}
		return strings.Join(a.RuleSet, ",") < strings.Join(b.RuleSet, ",")
	})
}

// mergeRulesAcrossUsers is shell-proxy's merge_user_template_compiled_rules:
// rules that differ only in who they apply to become one rule naming every
// such user, then rule-set rules that differ only in their rule sets are
// folded back together per family. Two users sending the same presets to the
// same chain get one rule rather than one each, and sing-box evaluates one
// match instead of two.
//
// It runs in three steps, as shell-proxy's does:
//  1. a rule-set rule carrying several tags is split into one rule per tag;
//  2. rules identical but for auth_user are merged, their users unioned;
//  3. rule-set rules identical but for their tags, within one family
//     (geosite, geoip, other), are merged back into one.
//
// The merged rules are then ordered by mergeRouteRulesByOutbound, which sorts
// each family as shell-proxy does.
func mergeRulesAcrossUsers(rules []store.RouteRule) []store.RouteRule {
	split := make([]store.RouteRule, 0, len(rules))
	for _, rule := range rules {
		rule.AuthUser = sortedUnique(rule.AuthUser)
		if !isPureRuleSetRouteRule(rule) || len(rule.RuleSet) < 2 {
			split = append(split, rule)
			continue
		}
		for _, tag := range uniqueStrings(rule.RuleSet) {
			single := rule
			single.RuleSet = []string{tag}
			split = append(split, single)
		}
	}

	byRule := map[string]int{}
	users := make([]store.RouteRule, 0, len(split))
	for _, rule := range split {
		key := routeRuleKey(rule, true, false)
		if at, ok := byRule[key]; ok {
			users[at].AuthUser = sortedUnique(append(users[at].AuthUser, rule.AuthUser...))
			continue
		}
		byRule[key] = len(users)
		rule.AuthUser = append([]string(nil), rule.AuthUser...)
		users = append(users, rule)
	}

	bySet := map[string]int{}
	merged := make([]store.RouteRule, 0, len(users))
	for _, rule := range users {
		if !isPureRuleSetRouteRule(rule) {
			merged = append(merged, rule)
			continue
		}
		key := routeRuleKey(rule, false, true) + "\x00" + strconv.Itoa(routeRuleClass(rule))
		if at, ok := bySet[key]; ok {
			merged[at].RuleSet = uniqueStrings(append(merged[at].RuleSet, rule.RuleSet...))
			continue
		}
		bySet[key] = len(merged)
		rule.RuleSet = append([]string(nil), rule.RuleSet...)
		merged = append(merged, rule)
	}
	return merged
}

// routeRuleKey identifies a rule for merging, leaving out its users or its
// rule sets. Slices are part of the identity in their stored order except
// auth_user, which is already sorted.
func routeRuleKey(rule store.RouteRule, withoutUsers, withoutRuleSets bool) string {
	if withoutUsers {
		rule.AuthUser = nil
	}
	if withoutRuleSets {
		rule.RuleSet = nil
	}
	encoded, _ := json.Marshal(rule)
	return string(encoded)
}

func sortedUnique(values []string) []string {
	out := uniqueStrings(values)
	sort.Strings(out)
	return out
}
