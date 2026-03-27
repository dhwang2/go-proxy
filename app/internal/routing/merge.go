package routing

import (
	"sort"
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
		out = append(out, b...)
	}
	return out
}
