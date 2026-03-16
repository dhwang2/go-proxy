package derived

import "go-proxy/internal/store"

// DNSRulesFromRoutes builds sing-box DNS rules from user route rules.
// Each route rule with auth_user produces a corresponding DNS rule
// that routes DNS queries through the appropriate server.
func DNSRulesFromRoutes(s *store.Store, outboundToDNS map[string]string, defaultStrategy string) []store.DNSRule {
	var rules []store.DNSRule
	for _, r := range s.UserRoutes {
		if len(r.AuthUser) == 0 {
			continue
		}
		server, ok := outboundToDNS[r.Outbound]
		if !ok {
			continue
		}
		rule := store.DNSRule{
			Action:        "route",
			Server:        server,
			AuthUser:      r.AuthUser,
			RuleSet:       r.RuleSet,
			Domain:        r.Domain,
			DomainSuffix:  r.DomainSuffix,
			DomainKeyword: r.DomainKeyword,
			DomainRegex:   r.DomainRegex,
		}
		if defaultStrategy != "" {
			rule.Strategy = defaultStrategy
		}
		rules = append(rules, rule)
	}
	return rules
}

// PruneOrphanAuthUsers removes auth_user entries from route and DNS rules
// for users that no longer exist.
func PruneOrphanAuthUsers(s *store.Store, activeUsers map[string]bool) bool {
	changed := false

	// Prune from user-route-rules.
	var kept []store.UserRouteRule
	for _, r := range s.UserRoutes {
		origLen := len(r.AuthUser)
		var validUsers []string
		for _, u := range r.AuthUser {
			if activeUsers[u] {
				validUsers = append(validUsers, u)
			}
		}
		if len(validUsers) > 0 {
			if len(validUsers) != origLen {
				changed = true
			}
			r.AuthUser = validUsers
			kept = append(kept, r)
		} else if origLen > 0 {
			changed = true
		}
	}
	s.UserRoutes = kept

	// Prune from sing-box route rules.
	if s.SingBox.Route != nil {
		var keptRoute []store.RouteRule
		for _, r := range s.SingBox.Route.Rules {
			if len(r.AuthUser) == 0 {
				keptRoute = append(keptRoute, r)
				continue
			}
			var validUsers []string
			for _, u := range r.AuthUser {
				if activeUsers[u] {
					validUsers = append(validUsers, u)
				}
			}
			if len(validUsers) > 0 {
				r.AuthUser = validUsers
				keptRoute = append(keptRoute, r)
			} else {
				changed = true
			}
		}
		s.SingBox.Route.Rules = keptRoute
	}

	// Prune from sing-box DNS rules.
	if s.SingBox.DNS != nil {
		var keptDNS []store.DNSRule
		for _, r := range s.SingBox.DNS.Rules {
			if len(r.AuthUser) == 0 {
				keptDNS = append(keptDNS, r)
				continue
			}
			var validUsers []string
			for _, u := range r.AuthUser {
				if activeUsers[u] {
					validUsers = append(validUsers, u)
				}
			}
			if len(validUsers) > 0 {
				r.AuthUser = validUsers
				keptDNS = append(keptDNS, r)
			} else {
				changed = true
			}
		}
		s.SingBox.DNS.Rules = keptDNS
	}

	return changed
}
