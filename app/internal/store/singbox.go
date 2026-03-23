package store

import (
	"encoding/json"

	"go-proxy/internal/config"
)

// SingBoxConfig is the top-level sing-box configuration.
type SingBoxConfig struct {
	Log          *LogConfig        `json:"log,omitempty"`
	DNS          *DNSConfig        `json:"dns,omitempty"`
	Inbounds     []Inbound         `json:"inbounds,omitempty"`
	Outbounds    []json.RawMessage `json:"outbounds,omitempty"`
	Route        *RouteConfig      `json:"route,omitempty"`
	Experimental json.RawMessage   `json:"experimental,omitempty"`
}

// EnsureDefaultDomainResolver sets route.default_domain_resolver if missing.
// Uses dns.final if available, otherwise the first DNS server tag.
func (c *SingBoxConfig) EnsureDefaultDomainResolver() {
	if c.Route == nil {
		return
	}
	if c.Route.DefaultDomainResolver != "" {
		return
	}
	// Prefer dns.final (matches shell-proxy behavior).
	if c.DNS != nil && c.DNS.Final != "" {
		c.Route.DefaultDomainResolver = c.DNS.Final
		return
	}
	// Fall back to the first DNS server tag.
	if c.DNS != nil {
		if tag := c.DNS.FirstServerTag(); tag != "" {
			c.Route.DefaultDomainResolver = tag
			return
		}
	}
}

// LogConfig configures sing-box logging.
type LogConfig struct {
	Disabled  bool   `json:"disabled"`
	Level     string `json:"level,omitempty"`
	Output    string `json:"output,omitempty"`
	Timestamp bool   `json:"timestamp,omitempty"`
}

// DNSConfig holds sing-box DNS configuration.
type DNSConfig struct {
	Servers          []json.RawMessage `json:"servers,omitempty"`
	Rules            []DNSRule         `json:"rules,omitempty"`
	Final            string            `json:"final,omitempty"`
	Strategy         string            `json:"strategy,omitempty"`
	ReverseMapping   bool              `json:"reverse_mapping,omitempty"`
	IndependentCache bool              `json:"independent_cache,omitempty"`
	CacheCapacity    int               `json:"cache_capacity,omitempty"`
}

// dnsServerFieldsToStrip lists fields that sing-box 1.13.x rejects inside
// individual dns.servers entries (they belong at .dns level or in dns rules).
var dnsServerFieldsToStrip = []string{"strategy", "client_subnet"}

// CleanDNSServers removes fields from individual DNS server entries that are
// invalid in sing-box 1.13.x (e.g. strategy, client_subnet).
func (c *SingBoxConfig) CleanDNSServers() {
	if c.DNS == nil || len(c.DNS.Servers) == 0 {
		return
	}
	for i, raw := range c.DNS.Servers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		changed := false
		for _, field := range dnsServerFieldsToStrip {
			if _, ok := m[field]; ok {
				delete(m, field)
				changed = true
			}
		}
		if changed {
			if cleaned, err := json.Marshal(m); err == nil {
				c.DNS.Servers[i] = cleaned
			}
		}
	}
}

// FirstServerTag returns the tag of the first DNS server, or empty string.
func (d *DNSConfig) FirstServerTag() string {
	if d == nil || len(d.Servers) == 0 {
		return ""
	}
	var srv struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(d.Servers[0], &srv); err != nil {
		return ""
	}
	return srv.Tag
}

// DNSRule is a sing-box DNS routing rule.
type DNSRule struct {
	Action        string   `json:"action,omitempty"`
	Server        string   `json:"server,omitempty"`
	Strategy      string   `json:"strategy,omitempty"`
	AuthUser      []string `json:"auth_user,omitempty"`
	Inbound       []string `json:"inbound,omitempty"`
	RuleSet       []string `json:"rule_set,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
}

// Inbound is a sing-box inbound configuration.
// Fields are a superset across all protocol types; unused fields are omitted via omitempty.
type Inbound struct {
	Type       string     `json:"type"`
	Tag        string     `json:"tag"`
	Listen     string     `json:"listen,omitempty"`
	ListenPort int        `json:"listen_port,omitempty"`
	Users      []User     `json:"users,omitempty"`
	TLS        *TLSConfig `json:"tls,omitempty"`

	// Shadowsocks-specific fields (inbound-level for single-user or server key).
	Method   string `json:"method,omitempty"`
	Password string `json:"password,omitempty"`

	// TUIC-specific.
	CongestionControl string `json:"congestion_control,omitempty"`
}

// User is a user entry within a sing-box inbound.
type User struct {
	Name     string `json:"name"`
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Flow     string `json:"flow,omitempty"`
}

// Credential returns the primary credential (UUID if set, else Password).
func (u User) Credential() string {
	if u.UUID != "" {
		return u.UUID
	}
	return u.Password
}

// FindUser returns the user with the given name, or nil.
func (ib *Inbound) FindUser(name string) *User {
	for i := range ib.Users {
		if ib.Users[i].Name == name {
			return &ib.Users[i]
		}
	}
	return nil
}

// ServerName returns the TLS server name, or empty string if no TLS.
func (ib *Inbound) ServerName() string {
	if ib.TLS != nil {
		return ib.TLS.ServerName
	}
	return ""
}

// HasReality returns whether this inbound uses Reality TLS.
func (ib *Inbound) HasReality() bool {
	return ib.TLS != nil && ib.TLS.Reality != nil && ib.TLS.Reality.Enabled
}

// TLSConfig holds TLS settings for an inbound.
type TLSConfig struct {
	Enabled         bool           `json:"enabled,omitempty"`
	ServerName      string         `json:"server_name,omitempty"`
	ALPN            []string       `json:"alpn,omitempty"`
	CertificatePath string         `json:"certificate_path,omitempty"`
	KeyPath         string         `json:"key_path,omitempty"`
	Reality         *RealityConfig `json:"reality,omitempty"`
}

// RealityConfig holds Reality TLS settings.
type RealityConfig struct {
	Enabled    bool              `json:"enabled,omitempty"`
	Handshake  *RealityHandshake `json:"handshake,omitempty"`
	PrivateKey string            `json:"private_key,omitempty"`
	ShortID    []string          `json:"short_id,omitempty"`
}

// RealityHandshake holds the Reality handshake target.
type RealityHandshake struct {
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
}

// OutboundHeader contains only the fields we need to inspect from outbounds.
// Full outbound JSON is preserved as json.RawMessage in SingBoxConfig.Outbounds.
type OutboundHeader struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

// ParseOutboundHeader extracts type and tag from a raw outbound JSON.
func ParseOutboundHeader(raw json.RawMessage) (OutboundHeader, error) {
	var h OutboundHeader
	err := json.Unmarshal(raw, &h)
	return h, err
}

// RouteConfig holds sing-box route configuration.
type RouteConfig struct {
	DefaultDomainResolver string            `json:"default_domain_resolver,omitempty"`
	Final                 string            `json:"final,omitempty"`
	Rules                 []RouteRule       `json:"rules,omitempty"`
	RuleSet               []json.RawMessage `json:"rule_set,omitempty"`
}

// RouteRule is a sing-box route rule.
type RouteRule struct {
	Action        string   `json:"action,omitempty"`
	Outbound      string   `json:"outbound,omitempty"`
	Protocol      string   `json:"protocol,omitempty"`
	AuthUser      []string `json:"auth_user,omitempty"`
	Inbound       []string `json:"inbound,omitempty"`
	RuleSet       []string `json:"rule_set,omitempty"`
	Sniffer       []string `json:"sniffer,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	IPIsPrivate   bool     `json:"ip_is_private,omitempty"`
}

// Normalize fills the shell-proxy baseline sections that go-proxy expects to exist.
func (c *SingBoxConfig) Normalize() {
	if c == nil {
		return
	}

	if c.Log == nil {
		c.Log = &LogConfig{
			Disabled:  false,
			Level:     "error",
			Output:    config.SingBoxLog,
			Timestamp: true,
		}
	} else {
		if c.Log.Level == "" {
			c.Log.Level = "error"
		}
		if c.Log.Output == "" {
			c.Log.Output = config.SingBoxLog
		}
	}

	if len(c.Experimental) == 0 {
		if raw, err := json.Marshal(config.DefaultExperimentalConfig()); err == nil {
			c.Experimental = raw
		}
	}

	if c.DNS == nil {
		c.DNS = &DNSConfig{}
	}
	defaultServers := rawMessagesFromMaps(config.DefaultDNSServers())
	if len(c.DNS.Servers) == 0 {
		c.DNS.Servers = defaultServers
	} else {
		c.DNS.Servers = appendMissingTaggedRaw(c.DNS.Servers, defaultServers)
	}
	c.CleanDNSServers()
	if c.DNS.Final == "" || c.DNS.Final == "dns-direct" {
		c.DNS.Final = "public4"
	}
	if c.DNS.Strategy == "" || c.DNS.Strategy == "ipv4_only" {
		c.DNS.Strategy = "prefer_ipv4"
	}
	if c.DNS.CacheCapacity == 0 {
		c.DNS.CacheCapacity = 8192
	}
	if !c.DNS.ReverseMapping {
		c.DNS.ReverseMapping = true
	}
	if !c.DNS.IndependentCache {
		c.DNS.IndependentCache = true
	}

	if len(c.Outbounds) == 0 {
		c.Outbounds = defaultDirectOutbounds()
	} else {
		c.Outbounds = normalizeOutbounds(c.Outbounds)
	}

	if c.Route == nil {
		c.Route = &RouteConfig{}
	}
	if c.Route.Final == "" || c.Route.Final == "direct" {
		c.Route.Final = "🐸 direct"
	}
	defaultRuleSets := rawMessagesFromMaps(config.DefaultRuleSetCatalog())
	if len(c.Route.RuleSet) == 0 {
		c.Route.RuleSet = defaultRuleSets
	} else {
		c.Route.RuleSet = appendMissingTaggedRaw(normalizeRuleSetCatalog(c.Route.RuleSet), defaultRuleSets)
	}
	c.Route.Rules = ensureBaseRouteRules(c.Route.Rules)
	c.EnsureDefaultDomainResolver()
}

func rawMessagesFromMaps(items []map[string]any) []json.RawMessage {
	raws := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			continue
		}
		raws = append(raws, raw)
	}
	return raws
}

func appendMissingTaggedRaw(existing, defaults []json.RawMessage) []json.RawMessage {
	seen := make(map[string]bool)
	for _, raw := range existing {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err == nil && item.Tag != "" {
			seen[item.Tag] = true
		}
	}
	out := append([]json.RawMessage(nil), existing...)
	for _, raw := range defaults {
		var item struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &item); err != nil || item.Tag == "" || seen[item.Tag] {
			continue
		}
		out = append(out, raw)
		seen[item.Tag] = true
	}
	return out
}

func defaultDirectOutbounds() []json.RawMessage {
	raw, err := json.Marshal(map[string]any{
		"type": "direct",
		"tag":  "🐸 direct",
	})
	if err != nil {
		return nil
	}
	return []json.RawMessage{raw}
}

func normalizeOutbounds(outbounds []json.RawMessage) []json.RawMessage {
	hasDirect := false
	for i, raw := range outbounds {
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if tag, _ := item["tag"].(string); tag == "direct" {
			item["tag"] = "🐸 direct"
			if normalized, err := json.Marshal(item); err == nil {
				outbounds[i] = normalized
			}
			hasDirect = true
			continue
		}
		if tag, _ := item["tag"].(string); tag == "🐸 direct" {
			hasDirect = true
		}
	}
	if hasDirect {
		return outbounds
	}
	return append(outbounds, defaultDirectOutbounds()...)
}

func normalizeRuleSetCatalog(ruleSets []json.RawMessage) []json.RawMessage {
	for i, raw := range ruleSets {
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if detour, _ := item["download_detour"].(string); detour == "direct" {
			item["download_detour"] = "🐸 direct"
			if normalized, err := json.Marshal(item); err == nil {
				ruleSets[i] = normalized
			}
		}
	}
	return ruleSets
}

func ensureBaseRouteRules(rules []RouteRule) []RouteRule {
	hasSniff := false
	hasHijackDNS := false
	hasPrivateDirect := false

	for i := range rules {
		if rules[i].Outbound == "direct" {
			rules[i].Outbound = "🐸 direct"
		}
		if rules[i].Action == "sniff" {
			hasSniff = true
		}
		if rules[i].Action == "hijack-dns" && rules[i].Protocol == "dns" {
			hasHijackDNS = true
		}
		if rules[i].Action == "route" && rules[i].IPIsPrivate && rules[i].Outbound == "🐸 direct" {
			hasPrivateDirect = true
		}
	}

	var base []RouteRule
	if !hasSniff {
		base = append(base, RouteRule{
			Action:  "sniff",
			Sniffer: []string{"http", "tls", "quic", "dns"},
		})
	}
	if !hasHijackDNS {
		base = append(base, RouteRule{
			Protocol: "dns",
			Action:   "hijack-dns",
		})
	}
	if !hasPrivateDirect {
		base = append(base, RouteRule{
			Action:      "route",
			Outbound:    "🐸 direct",
			IPIsPrivate: true,
		})
	}
	if len(base) == 0 {
		return rules
	}
	return append(base, rules...)
}
