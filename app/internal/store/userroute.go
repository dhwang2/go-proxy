package store

// UserRouteRule is one user's preset rule from user-route-rules.json: the
// preset's rule sets and the outbound they go to. routing compiles these into
// the sing-box route and DNS rules.
type UserRouteRule struct {
	Outbound string   `json:"outbound"`
	AuthUser []string `json:"auth_user,omitempty"`
	RuleSet  []string `json:"rule_set,omitempty"`
}
