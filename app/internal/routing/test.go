package routing

import (
	"net"
	"regexp"
	"strings"

	"go-proxy/internal/store"
)

// TestResult describes which rules match a given domain or IP for a user.
type TestResult struct {
	MatchedRules []MatchedRule `json:"matched_rules"`
	Unresolved   []MatchedRule `json:"unresolved_rule_sets"`
}

// MatchedRule describes a single rule that matched.
type MatchedRule struct {
	Outbound string `json:"outbound"`
	MatchBy  string `json:"match_by"`
	Value    string `json:"value"`
}

// TestDomain evaluates which routing rules would match a domain for a user.
// This is a dry-run evaluation — it does not modify any state.
func TestDomain(s *store.Store, userName, domain string) TestResult {
	matches := []MatchedRule{}
	unresolved := []MatchedRule{}
	for _, r := range s.UserRoutes {
		if !hasAuthUser(r.AuthUser, userName) {
			continue
		}
		// Check exact domain match.
		for _, d := range r.Domain {
			if d == domain {
				matches = append(matches, MatchedRule{
					Outbound: r.Outbound, MatchBy: "domain", Value: d,
				})
			}
		}
		// Check domain suffix match.
		for _, suffix := range r.DomainSuffix {
			if domainMatchesSuffix(domain, suffix) {
				matches = append(matches, MatchedRule{
					Outbound: r.Outbound, MatchBy: "domain_suffix", Value: suffix,
				})
			}
		}
		// Check domain keyword match.
		for _, kw := range r.DomainKeyword {
			if len(kw) > 0 && strings.Contains(domain, kw) {
				matches = append(matches, MatchedRule{
					Outbound: r.Outbound, MatchBy: "domain_keyword", Value: kw,
				})
			}
		}
		// Rule set matches can't be evaluated locally; note them.
		for _, rs := range r.RuleSet {
			unresolved = append(unresolved, MatchedRule{
				Outbound: r.Outbound, MatchBy: "rule_set", Value: rs,
			})
		}
		for _, pattern := range r.DomainRegex {
			if matched, err := regexp.MatchString(pattern, domain); err == nil && matched {
				matches = append(matches, MatchedRule{Outbound: r.Outbound, MatchBy: "domain_regex", Value: pattern})
			}
		}
		if ip := net.ParseIP(domain); ip != nil {
			for _, cidr := range r.IPCIDR {
				if _, prefix, err := net.ParseCIDR(cidr); err == nil && prefix.Contains(ip) {
					matches = append(matches, MatchedRule{Outbound: r.Outbound, MatchBy: "ip_cidr", Value: cidr})
				}
			}
		}
	}
	return TestResult{MatchedRules: matches, Unresolved: unresolved}
}

func hasAuthUser(users []string, name string) bool {
	for _, u := range users {
		if u == name {
			return true
		}
	}
	return false
}

func domainMatchesSuffix(domain, suffix string) bool {
	if domain == suffix {
		return true
	}
	if len(domain) > len(suffix) && domain[len(domain)-len(suffix)-1] == '.' {
		return domain[len(domain)-len(suffix):] == suffix
	}
	return false
}
