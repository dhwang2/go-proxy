package store

// UserRouteRule is a routing rule from user-route-rules.json.
// These are the source rules that get compiled into sing-box route/DNS rules.
type UserRouteRule struct {
	Action        string   `json:"action,omitempty"`
	Outbound      string   `json:"outbound"`
	AuthUser      []string `json:"auth_user,omitempty"`
	RuleSet       []string `json:"rule_set,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
}
