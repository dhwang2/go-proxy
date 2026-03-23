package routing

import (
	"encoding/json"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// SyncDNS rebuilds sing-box DNS rules and servers from user route rules.
// It replaces all auth_user-based DNS rules while preserving non-user rules.
// It also ensures each chain proxy outbound has a corresponding DNS server.
func SyncDNS(s *store.Store, outboundToDNS map[string]string, strategy string) {
	if s.SingBox.DNS == nil {
		return
	}

	// Build outbound-to-DNS mapping from chain proxy outbounds.
	if outboundToDNS == nil {
		outboundToDNS = buildOutboundToDNS(s)
	}

	// Sync DNS servers: ensure each chain outbound has a detour DNS server.
	syncChainDNSServers(s)

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
	s.SingBox.EnsureDefaultDomainResolver()
	s.MarkDirty(store.FileSingBox)
}

// buildOutboundToDNS creates a mapping from outbound tags to DNS server tags.
// Chain proxy outbounds (res-socks*) map to their corresponding DNS tags (res-proxy*).
func buildOutboundToDNS(s *store.Store) map[string]string {
	directDNS := "public4"
	if s.SingBox.Route != nil && s.SingBox.Route.DefaultDomainResolver != "" {
		directDNS = s.SingBox.Route.DefaultDomainResolver
	} else if s.SingBox.DNS != nil && s.SingBox.DNS.Final != "" {
		directDNS = s.SingBox.DNS.Final
	}
	m := map[string]string{
		"direct":   directDNS,
		"🐸 direct": directDNS,
	}
	for _, raw := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(raw)
		if IsChainTag(h.Tag) {
			m[h.Tag] = ChainDNSTag(h.Tag)
		}
	}
	return m
}

// chainDNSServer represents a DNS-over-HTTPS server entry for chain proxy outbounds.
type chainDNSServer struct {
	Tag        string         `json:"tag"`
	Type       string         `json:"type"`
	Server     string         `json:"server"`
	ServerPort int            `json:"server_port"`
	Path       string         `json:"path"`
	TLS        map[string]any `json:"tls"`
	Detour     string         `json:"detour"`
	Strategy   string         `json:"strategy,omitempty"`
}

// syncChainDNSServers ensures each chain proxy outbound has a corresponding DNS server.
// Removes stale chain DNS servers and adds missing ones.
func syncChainDNSServers(s *store.Store) {
	if s.SingBox.DNS == nil {
		return
	}

	// Collect current chain outbound tags.
	chainTags := make(map[string]bool)
	for _, raw := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(raw)
		if IsChainTag(h.Tag) {
			chainTags[h.Tag] = true
		}
	}

	// Filter out stale chain DNS servers, keep non-chain servers.
	var kept []json.RawMessage
	existingDNS := make(map[string]bool)
	for _, raw := range s.SingBox.DNS.Servers {
		var srv struct {
			Tag    string `json:"tag"`
			Detour string `json:"detour"`
		}
		if err := json.Unmarshal(raw, &srv); err != nil {
			kept = append(kept, raw)
			continue
		}
		// If this is a chain DNS server, only keep it if the outbound still exists.
		if IsChainTag(srv.Detour) {
			if chainTags[srv.Detour] {
				kept = append(kept, raw)
				existingDNS[srv.Tag] = true
			}
			continue
		}
		kept = append(kept, raw)
		existingDNS[srv.Tag] = true
	}

	// Add DNS servers for chain outbounds that don't have one yet.
	for tag := range chainTags {
		dnsTag := ChainDNSTag(tag)
		if existingDNS[dnsTag] {
			continue
		}
		srv := chainDNSServer{
			Tag:        dnsTag,
			Type:       "https",
			Server:     "8.8.8.8",
			ServerPort: 443,
			Path:       "/dns-query",
			TLS: map[string]any{
				"enabled":     true,
				"server_name": "dns.google",
			},
			Detour: tag,
		}
		raw, err := json.Marshal(srv)
		if err != nil {
			continue
		}
		kept = append(kept, raw)
	}

	s.SingBox.DNS.Servers = kept
}
