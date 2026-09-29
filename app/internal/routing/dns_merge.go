package routing

import (
	"fmt"
	"strings"

	"go-proxy/internal/store"
)

func mergeDNSRulesByServer(rules []dnsRoute) []dnsRoute {
	type key struct {
		server   string
		strategy string
		users    string
		class    int
	}

	index := make(map[key]int)
	merged := make([]dnsRoute, 0, len(rules))

	for _, r := range rules {
		cls := dnsRuleClass(r.rule)
		if (cls != 1 && cls != 2) || !isPureRuleSetDNSRule(r.rule) {
			merged = append(merged, r)
			continue
		}
		k := key{
			server:   r.rule.Server,
			strategy: r.strategy,
			users:    authUserKey(r.rule.AuthUser),
			class:    cls,
		}
		if pos, exists := index[k]; exists {
			merged[pos].rule.RuleSet = uniqueStrings(append(merged[pos].rule.RuleSet, r.rule.RuleSet...))
			continue
		}
		index[k] = len(merged)
		cp := r
		cp.rule.RuleSet = append([]string(nil), r.rule.RuleSet...)
		merged = append(merged, cp)
	}

	var buckets [4][]dnsRoute
	for _, r := range merged {
		buckets[dnsRuleClass(r.rule)] = append(buckets[dnsRuleClass(r.rule)], r)
	}
	out := make([]dnsRoute, 0, len(merged))
	for _, bucket := range buckets {
		out = append(out, bucket...)
	}
	return out
}

// dnsRulesFor gives each rule its sing-box 1.14 form. A rule on address sets
// (geoip rule-sets) cannot match the query, only an answer: the query goes to
// the rule's server with evaluate, and a rule matching that answer against
// the sets returns it with respond; unmatched, lookup carries on. A chain
// that reaches one address family keeps its lookups to that family: its
// domain rules answer the other family's queries with an empty answer, and
// its address rules evaluate only its own family's queries.
func dnsRulesFor(routes []dnsRoute) []store.DNSRule {
	out := make([]store.DNSRule, 0, len(routes))
	for index, route := range routes {
		keep, drop := queryTypes(route.strategy)
		var names, addresses []string
		for _, set := range route.rule.RuleSet {
			if strings.HasPrefix(set, "geoip-") {
				addresses = append(addresses, set)
			} else {
				names = append(names, set)
			}
		}
		if len(addresses) > 0 {
			tag := fmt.Sprintf("%s-geoip-%d", route.rule.Server, index)
			evaluate := store.DNSRule{Action: "evaluate", Server: route.rule.Server, Tag: tag, AuthUser: route.rule.AuthUser}
			if keep != "" {
				evaluate.QueryType = []string{keep}
			}
			out = append(out, evaluate, store.DNSRule{Action: "respond", AuthUser: route.rule.AuthUser, RuleSet: addresses, MatchResponse: tag})
		}
		rule := route.rule
		rule.RuleSet = names
		if len(rule.RuleSet) == 0 && len(rule.Domain) == 0 && len(rule.DomainSuffix) == 0 && len(rule.DomainKeyword) == 0 && len(rule.DomainRegex) == 0 {
			continue
		}
		if drop != "" {
			empty := rule
			empty.Action, empty.Server, empty.QueryType = "predefined", "", []string{drop}
			out = append(out, empty)
		}
		out = append(out, rule)
	}
	return out
}

// queryTypes is the query type a strategy keeps and the one it answers
// empty; both empty when either family will do.
func queryTypes(strategy string) (keep, drop string) {
	switch strategy {
	case "ipv4_only":
		return "A", "AAAA"
	case "ipv6_only":
		return "AAAA", "A"
	}
	return "", ""
}

func dnsRuleClass(r store.DNSRule) int {
	if len(r.RuleSet) == 0 {
		if len(r.Domain) > 0 || len(r.DomainSuffix) > 0 || len(r.DomainKeyword) > 0 || len(r.DomainRegex) > 0 {
			return 0
		}
		return 3
	}
	if isGeositeRuleSet(r.RuleSet) {
		return 1
	}
	if isGeoIPRuleSet(r.RuleSet) {
		return 2
	}
	return 3
}

func isPureRuleSetDNSRule(r store.DNSRule) bool {
	return len(r.RuleSet) > 0 &&
		len(r.Domain) == 0 &&
		len(r.DomainSuffix) == 0 &&
		len(r.DomainKeyword) == 0 &&
		len(r.DomainRegex) == 0
}
