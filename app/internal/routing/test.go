package routing

import (
	"encoding/json"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"go-proxy/internal/store"
)

// RuleSetMatcher reports which of tags, if any, holds target. Unchecked names
// the tags it could not look into, a rule set sing-box has not cached yet; a
// rule that needs one of them is undecided rather than unmatched.
type RuleSetMatcher func(tags []string) (matched string, unchecked []string, err error)

// Evaluation is where one connection would leave: the compiled rule that
// takes it, or the route final.
type Evaluation struct {
	Target string `json:"target"`
	// Kind is "domain" or "ip". A domain is evaluated as the name a client
	// asked for: IP rules -- geoip sets, ip_cidr, private ranges -- do not
	// decide it, as they do not in sing-box without a resolve action.
	Kind     string   `json:"kind"`
	Decision Decision `json:"decision"`
	// Unchecked names rule sets that rules before the decision need and
	// sing-box has not cached. While any is listed the decision holds only if
	// none of them contains the target.
	Unchecked []string `json:"unchecked_rule_sets,omitempty"`
}

// Decision is the rule that takes the connection. It carries where the traffic
// goes and where the name is resolved, because the two are the whole answer: a
// rule that egresses through a residential address but resolves somewhere else
// leaks the lookup and can return the wrong address.
type Decision struct {
	// Rule is the position in route.rules, -1 for the route final.
	Rule int `json:"rule"`
	// MatchBy is the item that matched -- domain, domain_suffix,
	// domain_keyword, domain_regex, ip_cidr, ip_is_private, rule_set -- or
	// auth_user for a rule that takes all of a user's traffic, or final.
	MatchBy  string `json:"match_by"`
	Value    string `json:"value,omitempty"`
	Outbound string `json:"outbound"`
	// DNSServer is the sing-box DNS server this rule's lookups use, and DNSVia
	// the outbound that server itself egresses through. DNSVia equal to
	// Outbound is the synchronised case; empty means the resolver goes out
	// directly, which is correct only for a direct rule.
	DNSServer string `json:"dns_server,omitempty"`
	DNSVia    string `json:"dns_via"`
}

// Evaluate walks the compiled route rules in order, as sing-box does, and
// stops at the first that takes a connection from user to target. It reads
// the compiled rules rather than the stored user rules: they are what runs,
// merged across users, and a preset whose rule set is available compiles
// without its fallback domains.
func Evaluate(s *store.Store, user, target string, match RuleSetMatcher) (Evaluation, error) {
	address, err := netip.ParseAddr(target)
	isIP := err == nil
	result := Evaluation{Target: target, Kind: "domain"}
	if isIP {
		result.Kind = "ip"
	}
	// The same mapping the compiler uses to build the DNS rules, so what this
	// reports is what the running configuration does rather than a second
	// opinion about it.
	toDNS := buildOutboundToDNS(s)
	detour := map[string]string{}
	if s.SingBox.DNS != nil {
		for _, raw := range s.SingBox.DNS.Servers {
			var server struct {
				Tag    string `json:"tag"`
				Detour string `json:"detour"`
			}
			if json.Unmarshal(raw, &server) == nil {
				detour[server.Tag] = server.Detour
			}
		}
	}
	// Labelled on the way out, looked up on the way in: the stored tag is what
	// indexes the DNS mapping, and the decorated internal spelling of direct is
	// not something a caller should have to recognise.
	decide := func(index int, by, value, outbound string) Decision {
		server := toDNS[outbound]
		return Decision{
			Rule: index, MatchBy: by, Value: value,
			Outbound: OutboundLabel(outbound), DNSServer: server, DNSVia: OutboundLabel(detour[server]),
		}
	}
	var rules []store.RouteRule
	final := store.DirectTag
	if s.SingBox.Route != nil {
		rules = s.SingBox.Route.Rules
		if s.SingBox.Route.Final != "" {
			final = s.SingBox.Route.Final
		}
	}
	for index, rule := range rules {
		// sniff and hijack-dns shape the connection without choosing an
		// outbound, and a rule keyed on protocol or inbound depends on what
		// the connection carries, which a name alone does not say.
		if rule.Action != "" && rule.Action != "route" || rule.Outbound == "" {
			continue
		}
		if rule.Protocol != "" || len(rule.Inbound) > 0 {
			continue
		}
		if len(rule.AuthUser) > 0 && !slices.Contains(rule.AuthUser, user) {
			continue
		}
		if by, value, ok := matchItems(rule, target, address, isIP); ok {
			result.Decision = decide(index, by, value, rule.Outbound)
			return result, nil
		}
		// The compiler keeps geosite and geoip sets in separate rules, named
		// by that prefix; the other kind cannot match this target.
		skip := "geoip-"
		if isIP {
			skip = "geosite-"
		}
		sets := []string{}
		for _, tag := range rule.RuleSet {
			if !strings.HasPrefix(tag, skip) {
				sets = append(sets, tag)
			}
		}
		if len(sets) > 0 {
			tag, unchecked, err := match(sets)
			if err != nil {
				return Evaluation{}, err
			}
			result.Unchecked = append(result.Unchecked, unchecked...)
			if tag != "" {
				result.Decision = decide(index, "rule_set", tag, rule.Outbound)
				return result, nil
			}
		}
		if !hasAddressItems(rule) {
			result.Decision = decide(index, "auth_user", "", rule.Outbound)
			return result, nil
		}
	}
	result.Decision = decide(-1, "final", "", final)
	return result, nil
}

// matchItems checks a rule's own address items, which sing-box ORs together:
// the name items against a domain, the address items against an IP.
func matchItems(rule store.RouteRule, domain string, address netip.Addr, isIP bool) (string, string, bool) {
	if isIP {
		if rule.IPIsPrivate && !publicAddress(address) {
			return "ip_is_private", "", true
		}
		for _, cidr := range rule.IPCIDR {
			if prefix, err := netip.ParsePrefix(cidr); err == nil && prefix.Contains(address) {
				return "ip_cidr", cidr, true
			}
			if single, err := netip.ParseAddr(cidr); err == nil && single == address {
				return "ip_cidr", cidr, true
			}
		}
		return "", "", false
	}
	for _, exact := range rule.Domain {
		if strings.EqualFold(exact, domain) {
			return "domain", exact, true
		}
	}
	for _, suffix := range rule.DomainSuffix {
		if domainMatchesSuffix(domain, strings.ToLower(suffix)) {
			return "domain_suffix", suffix, true
		}
	}
	for _, keyword := range rule.DomainKeyword {
		if keyword != "" && strings.Contains(domain, strings.ToLower(keyword)) {
			return "domain_keyword", keyword, true
		}
	}
	for _, pattern := range rule.DomainRegex {
		if matched, err := regexp.MatchString(pattern, domain); err == nil && matched {
			return "domain_regex", pattern, true
		}
	}
	return "", "", false
}

func hasAddressItems(rule store.RouteRule) bool {
	return len(rule.Domain)+len(rule.DomainSuffix)+len(rule.DomainKeyword)+len(rule.DomainRegex)+
		len(rule.IPCIDR)+len(rule.RuleSet) > 0 || rule.IPIsPrivate
}

// publicAddress is sing-box's test behind ip_is_private.
func publicAddress(address netip.Addr) bool {
	return !(address.IsPrivate() || address.IsLoopback() || address.IsMulticast() ||
		address.IsLinkLocalUnicast() || address.IsInterfaceLocalMulticast() || address.IsUnspecified())
}

// domainMatchesSuffix is sing-box's domain_suffix: "example.com" takes the
// name itself and every name under it, ".example.com" only the names under it.
func domainMatchesSuffix(domain, suffix string) bool {
	if strings.HasPrefix(suffix, ".") {
		return strings.HasSuffix(domain, suffix)
	}
	return domain == suffix || strings.HasSuffix(domain, "."+suffix)
}
