package routing

import (
	"encoding/json"
	"slices"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

// CompiledUserRouteRules is the users' rules in sing-box's form, as few as
// their choices allow: users sending one rule set to one outbound share a
// rule, and that rule carries every rule set of its family (domain sets, then
// address sets) those users send there. Rules are sorted by family, outbound,
// users and rule sets, so the same choices always compile to the same file.
func CompiledUserRouteRules(s *store.Store) []store.RouteRule {
	ensureRequiredRuleSetCatalog(s)
	available := availableRuleSetTags(s)

	type sent struct{ outbound, tag string }
	usersOf := map[sent][]string{}
	for _, rule := range s.UserRoutes {
		preset, ok := PresetOf(rule)
		if !ok {
			continue
		}
		for _, tag := range preset.RuleSets {
			if available[tag] {
				key := sent{rule.Outbound, tag}
				usersOf[key] = append(usersOf[key], rule.AuthUser...)
			}
		}
	}

	type group struct {
		outbound, users string
		geoip           bool
	}
	grouped := map[group]*store.RouteRule{}
	for key, users := range usersOf {
		users = sortedUnique(users)
		at := group{key.outbound, strings.Join(users, ","), isGeoIP(key.tag)}
		if grouped[at] == nil {
			grouped[at] = &store.RouteRule{Action: "route", Outbound: key.outbound, AuthUser: users}
		}
		grouped[at].RuleSet = append(grouped[at].RuleSet, key.tag)
	}
	rules := make([]store.RouteRule, 0, len(grouped))
	for _, rule := range grouped {
		slices.Sort(rule.RuleSet)
		rules = append(rules, *rule)
	}
	slices.SortFunc(rules, func(a, b store.RouteRule) int {
		if ga, gb := isGeoIP(a.RuleSet[0]), isGeoIP(b.RuleSet[0]); ga != gb {
			if ga {
				return 1
			}
			return -1
		}
		return strings.Compare(
			a.Outbound+"\x00"+strings.Join(a.AuthUser, ",")+"\x00"+strings.Join(a.RuleSet, ","),
			b.Outbound+"\x00"+strings.Join(b.AuthUser, ",")+"\x00"+strings.Join(b.RuleSet, ","))
	})
	return rules
}

func isGeoIP(tag string) bool { return strings.HasPrefix(tag, "geoip-") }

func sortedUnique(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

func availableRuleSetTags(s *store.Store) map[string]bool {
	tags := make(map[string]bool)
	if s == nil || s.SingBox == nil || s.SingBox.Route == nil {
		return tags
	}
	for _, raw := range s.SingBox.Route.RuleSet {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err == nil && item.Tag != "" {
			tags[item.Tag] = true
		}
	}
	return tags
}

// ensureRequiredRuleSetCatalog adds the catalogue entry of every rule set a
// stored rule needs and the configuration lacks, domain sets ahead of address
// sets.
func ensureRequiredRuleSetCatalog(s *store.Store) {
	if s == nil || s.SingBox == nil {
		return
	}
	if s.SingBox.Route == nil {
		s.SingBox.Route = &store.RouteConfig{}
	}
	present := availableRuleSetTags(s)
	catalog := make(map[string]json.RawMessage)
	for _, item := range config.DefaultRuleSetCatalog() {
		tag, _ := item["tag"].(string)
		if raw, err := json.Marshal(item); err == nil && tag != "" {
			catalog[tag] = raw
		}
	}
	added := false
	for _, rule := range s.UserRoutes {
		preset, ok := PresetOf(rule)
		if !ok {
			continue
		}
		for _, tag := range preset.RuleSets {
			if raw, known := catalog[tag]; known && !present[tag] {
				s.SingBox.Route.RuleSet = append(s.SingBox.Route.RuleSet, raw)
				present[tag] = true
				added = true
			}
		}
	}
	if !added {
		return
	}
	var domains, addresses []json.RawMessage
	for _, raw := range s.SingBox.Route.RuleSet {
		var item struct {
			Tag string `json:"tag"`
		}
		if json.Unmarshal(raw, &item) == nil && isGeoIP(item.Tag) {
			addresses = append(addresses, raw)
		} else {
			domains = append(domains, raw)
		}
	}
	s.SingBox.Route.RuleSet = append(domains, addresses...)
}

// dnsRoute is a DNS rule before it takes its sing-box form: where its queries
// go and the address families its server's chain can reach.
type dnsRoute struct {
	rule     store.DNSRule
	strategy string
}

// dnsRulesFromRouteRules sends each compiled rule's lookups where its traffic
// goes: the DNS server of its outbound, asking for what that server is
// configured to resolve. Only a chain carries its own strategy; direct keeps
// the configured one.
func dnsRulesFromRouteRules(routeRules []store.RouteRule, outboundToDNS map[string]string, serverStrategy map[string]string, defaultStrategy string) []dnsRoute {
	rules := make([]dnsRoute, 0, len(routeRules))
	for _, rule := range routeRules {
		server, ok := outboundToDNS[rule.Outbound]
		if !ok || len(rule.AuthUser) == 0 || len(rule.RuleSet) == 0 {
			continue
		}
		strategy := defaultStrategy
		if perServer, ok := serverStrategy[server]; ok {
			strategy = perServer
		}
		rules = append(rules, dnsRoute{rule: store.DNSRule{
			Action:   "route",
			Server:   server,
			AuthUser: slices.Clone(rule.AuthUser),
			RuleSet:  slices.Clone(rule.RuleSet),
		}, strategy: strategy})
	}
	return rules
}
