package routing

import "go-proxy/internal/store"

func mergeDNSRulesByServer(rules []store.DNSRule) []store.DNSRule {
	type key struct {
		server   string
		strategy string
		users    string
		class    int
	}

	index := make(map[key]int)
	merged := make([]store.DNSRule, 0, len(rules))

	for _, r := range rules {
		cls := dnsRuleClass(r)
		if (cls != 1 && cls != 2) || !isPureRuleSetDNSRule(r) {
			merged = append(merged, r)
			continue
		}
		k := key{
			server:   r.Server,
			strategy: r.Strategy,
			users:    authUserKey(r.AuthUser),
			class:    cls,
		}
		if pos, exists := index[k]; exists {
			merged[pos].RuleSet = uniqueStrings(append(merged[pos].RuleSet, r.RuleSet...))
			continue
		}
		index[k] = len(merged)
		cp := r
		cp.RuleSet = append([]string(nil), r.RuleSet...)
		merged = append(merged, cp)
	}

	var buckets [4][]store.DNSRule
	for _, r := range merged {
		buckets[dnsRuleClass(r)] = append(buckets[dnsRuleClass(r)], r)
	}
	out := make([]store.DNSRule, 0, len(merged))
	for _, bucket := range buckets {
		out = append(out, bucket...)
	}
	return out
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
