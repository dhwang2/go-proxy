package derived

import (
	"encoding/json"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

type DNSContext struct {
	OutboundServer map[string]string
	DefaultServer  string
}

func DNSRulesFromRoutes(routes []store.RouteRule, outbounds []store.Outbound, ctx DNSContext) []store.DNSRule {
	if len(routes) == 0 {
		return nil
	}
	serverByOutbound := map[string]string{}
	for _, ob := range outbounds {
		tag := strings.TrimSpace(ob.Tag)
		if tag == "" {
			continue
		}
		if v, ok := ctx.OutboundServer[tag]; ok && strings.TrimSpace(v) != "" {
			serverByOutbound[tag] = strings.TrimSpace(v)
		}
	}

	dedup := make(map[string]struct{}, len(routes))
	rules := make([]store.DNSRule, 0, len(routes))
	for _, r := range routes {
		if len(r.RuleSet) == 0 {
			continue
		}
		geositeRuleSet := filterGeositeRuleSets(r.RuleSet)
		if len(geositeRuleSet) == 0 {
			continue
		}
		server := strings.TrimSpace(serverByOutbound[r.Outbound])
		if server == "" {
			server = strings.TrimSpace(ctx.DefaultServer)
		}
		if server == "" {
			continue
		}
		d := store.DNSRule{
			Action:   "route",
			Server:   server,
			AuthUser: append([]string(nil), r.AuthUser...),
			RuleSet:  geositeRuleSet,
		}
		k := dedupKey(d)
		if _, ok := dedup[k]; ok {
			continue
		}
		dedup[k] = struct{}{}
		rules = append(rules, d)
	}
	return rules
}

func filterGeositeRuleSets(ruleSet []string) []string {
	out := make([]string, 0, len(ruleSet))
	for _, tag := range ruleSet {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(tag, "geosite-") {
			out = append(out, tag)
		}
	}
	return out
}

func dedupKey(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
