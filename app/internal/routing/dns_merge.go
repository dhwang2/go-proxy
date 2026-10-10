package routing

import (
	"fmt"
	"slices"
	"strings"

	"go-proxy/internal/store"
)

// mergeDNSRulesByServer folds rules that send the same users' lookups to the
// same server with the same strategy into one per family, domain sets ahead of
// address sets. Compiled rules each carry one family, so a rule's first set
// names its family.
func mergeDNSRulesByServer(rules []dnsRoute) []dnsRoute {
	type key struct {
		server, strategy, users string
		geoip                   bool
	}
	index := make(map[key]int)
	var families [2][]dnsRoute
	for _, r := range rules {
		geoip := isGeoIP(r.rule.RuleSet[0])
		family := 0
		if geoip {
			family = 1
		}
		k := key{r.rule.Server, r.strategy, strings.Join(r.rule.AuthUser, ","), geoip}
		if at, ok := index[k]; ok {
			merged := &families[family][at]
			for _, set := range r.rule.RuleSet {
				if !slices.Contains(merged.rule.RuleSet, set) {
					merged.rule.RuleSet = append(merged.rule.RuleSet, set)
				}
			}
			continue
		}
		index[k] = len(families[family])
		families[family] = append(families[family], r)
	}
	return append(families[0], families[1]...)
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
			if isGeoIP(set) {
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
		if len(rule.RuleSet) == 0 {
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
