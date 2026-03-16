package routing

import (
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// DefaultOutboundToDNS maps outbound tags to DNS server tags.
var DefaultOutboundToDNS = map[string]string{
	"res-socks": "res-proxy",
	"🐸 direct":  "dns-direct",
}

// SyncDNS rebuilds sing-box DNS rules from user route rules.
// It replaces all auth_user-based DNS rules while preserving non-user rules.
func SyncDNS(s *store.Store, outboundToDNS map[string]string, strategy string) {
	if s.SingBox.DNS == nil {
		return
	}
	if outboundToDNS == nil {
		outboundToDNS = DefaultOutboundToDNS
	}

	// Keep non-auth_user DNS rules.
	var kept []store.DNSRule
	for _, r := range s.SingBox.DNS.Rules {
		if len(r.AuthUser) == 0 {
			kept = append(kept, r)
		}
	}

	// Generate new auth_user DNS rules from user routes.
	newRules := derived.DNSRulesFromRoutes(s, outboundToDNS, strategy)
	kept = append(kept, newRules...)

	s.SingBox.DNS.Rules = kept
	s.MarkDirty(store.FileSingBox)
}

// SyncRouteRules rebuilds sing-box route rules from user route rules.
// It replaces all auth_user-based route rules while preserving non-user rules.
func SyncRouteRules(s *store.Store) {
	if s.SingBox.Route == nil {
		s.SingBox.Route = &store.RouteConfig{}
	}

	// Keep non-auth_user route rules.
	var kept []store.RouteRule
	for _, r := range s.SingBox.Route.Rules {
		if len(r.AuthUser) == 0 {
			kept = append(kept, r)
		}
	}

	// Convert user route rules to sing-box route rules.
	for _, ur := range s.UserRoutes {
		rr := store.RouteRule{
			Action:        ur.Action,
			Outbound:      ur.Outbound,
			AuthUser:      ur.AuthUser,
			RuleSet:       ur.RuleSet,
			Domain:        ur.Domain,
			DomainSuffix:  ur.DomainSuffix,
			DomainKeyword: ur.DomainKeyword,
			DomainRegex:   ur.DomainRegex,
			IPCIDR:        ur.IPCIDR,
		}
		kept = append(kept, rr)
	}

	s.SingBox.Route.Rules = kept
	s.MarkDirty(store.FileSingBox)
}
