package routing

import (
	"strings"

	"go-proxy/internal/store"
)

// TestResult describes which rules match a given domain or IP for a user.
type TestResult struct {
	MatchedRules []MatchedRule
}

// MatchedRule describes a single rule that matched.
type MatchedRule struct {
	Outbound string
	MatchBy  string // what field matched (domain, domain_suffix, rule_set, ip_cidr, etc.)
	Value    string // the specific value that matched
}

// TestDomain evaluates which routing rules would match a domain for a user.
// This is a dry-run evaluation — it does not modify any state.
func TestDomain(s *store.Store, userName, domain string) TestResult {
	var matches []MatchedRule
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
			matches = append(matches, MatchedRule{
				Outbound: r.Outbound, MatchBy: "rule_set", Value: rs,
			})
		}
	}
	return TestResult{MatchedRules: matches}
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
