package store

import (
	"encoding/json"
	"slices"

	"go-proxy/internal/config"
	"go-proxy/pkg/jsonorder"
)

// SingBoxConfig is the top-level sing-box configuration.
// The field order is the order sections are written in, and matches the
// shell-proxy layout: log, experimental, dns, inbounds, outbounds, route.
type SingBoxConfig struct {
	Log          *LogConfig        `json:"log,omitempty"`
	Experimental json.RawMessage   `json:"experimental,omitempty"`
	DNS          *DNSConfig        `json:"dns,omitempty"`
	Inbounds     []Inbound         `json:"inbounds,omitempty"`
	Outbounds    []json.RawMessage `json:"outbounds,omitempty"`
	Route        *RouteConfig      `json:"route,omitempty"`
}

// DirectTag is the tag of the direct outbound. Configurations written before
// it was plain carry LegacyDirectTag, which loading rewrites.
const (
	DirectTag       = "direct"
	LegacyDirectTag = "🐸 direct"
)

// DirectOutbound returns the tag, if any, a stored outbound name should be
// rewritten to: the legacy direct tag becomes the plain one.
func DirectOutbound(tag string) string {
	if tag == LegacyDirectTag {
		return DirectTag
	}
	return tag
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
		c.DNS.Servers[i] = editRaw(raw, func(v *jsonorder.Value) {
			keys, fields := v.Keys[:0:0], v.Fields[:0:0]
			for index, name := range v.Keys {
				if !slices.Contains(dnsServerFieldsToStrip, name) {
					keys = append(keys, name)
					fields = append(fields, v.Fields[index])
				}
			}
			v.Keys, v.Fields = keys, fields
		})
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
	Final                 string            `json:"final,omitempty"`
	DefaultDomainResolver string            `json:"default_domain_resolver,omitempty"`
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
	if c.Route.Final == "" || c.Route.Final == LegacyDirectTag {
		c.Route.Final = DirectTag
	}
	defaultRuleSets := rawMessagesFromMaps(config.DefaultRuleSetCatalog())
	if len(c.Route.RuleSet) == 0 {
		c.Route.RuleSet = defaultRuleSets
	} else {
		c.Route.RuleSet = appendMissingTaggedRaw(normalizeRuleSetCatalog(c.Route.RuleSet), defaultRuleSets)
	}
	c.Route.Rules = ensureBaseRouteRules(c.Route.Rules)
	c.EnsureDefaultDomainResolver()
	c.canonicalOrder()
}

// Key orders for the entries go-proxy writes. The defaults are built as maps,
// and encoding a map sorts its keys; these put them back in the order sing-box
// documents them, so the file reads the way shell-proxy wrote it. Keys not
// listed keep their place after these.
var (
	dnsServerOrder = []string{"tag", "type", "server", "server_port", "path", "tls", "detour", "domain_strategy"}
	outboundOrder  = []string{"type", "tag", "server", "server_port", "version", "udp_over_tcp", "username", "password", "domain_resolver"}
	ruleSetOrder   = []string{"tag", "type", "format", "url", "download_detour"}
	cacheFileOrder = []string{"enabled", "cache_id", "path", "store_fakeip", "store_rdrc"}
)

func (c *SingBoxConfig) canonicalOrder() {
	if len(c.Experimental) > 0 {
		c.Experimental = editRaw(c.Experimental, func(v *jsonorder.Value) {
			v.Get("cache_file").Reorder(cacheFileOrder...)
		})
	}
	if c.DNS != nil {
		for i, raw := range c.DNS.Servers {
			c.DNS.Servers[i] = editRaw(raw, func(v *jsonorder.Value) {
				v.Reorder(dnsServerOrder...)
				v.Get("tls").Reorder("enabled", "server_name")
			})
		}
	}
	for i, raw := range c.Outbounds {
		c.Outbounds[i] = editRaw(raw, func(v *jsonorder.Value) {
			v.Reorder(outboundOrder...)
			v.Get("domain_resolver").Reorder("server", "strategy")
		})
	}
	if c.Route != nil {
		for i, raw := range c.Route.RuleSet {
			c.Route.RuleSet[i] = editRaw(raw, func(v *jsonorder.Value) { v.Reorder(ruleSetOrder...) })
		}
	}
}

// editRaw applies edit to one raw JSON object, keeping its key order. A value
// that is not an object, or does not parse, is returned unchanged.
func editRaw(raw json.RawMessage, edit func(*jsonorder.Value)) json.RawMessage {
	value, err := jsonorder.Parse(raw)
	if err != nil || value.Kind != jsonorder.Object {
		return raw
	}
	edit(value)
	encoded, err := value.MarshalJSON()
	if err != nil {
		return raw
	}
	return encoded
}

// DirectResolver reports the resolver the direct outbound's own lookups use.
// A configuration from before go-proxy chose one per host has none.
func (c *SingBoxConfig) DirectResolver() (server, strategy string, ok bool) {
	for _, raw := range c.Outbounds {
		var outbound struct {
			Tag            string `json:"tag"`
			DomainResolver *struct {
				Server   string `json:"server"`
				Strategy string `json:"strategy"`
			} `json:"domain_resolver"`
		}
		if json.Unmarshal(raw, &outbound) != nil || outbound.Tag != DirectTag {
			continue
		}
		if outbound.DomainResolver == nil {
			return "", "", false
		}
		return outbound.DomainResolver.Server, outbound.DomainResolver.Strategy, true
	}
	return "", "", false
}

// SetDirectResolver points the direct outbound's lookups at server with
// strategy. An empty strategy leaves the family choice to sing-box.
func (c *SingBoxConfig) SetDirectResolver(server, strategy string) {
	for i, raw := range c.Outbounds {
		var header OutboundHeader
		if json.Unmarshal(raw, &header) != nil || header.Tag != DirectTag {
			continue
		}
		c.Outbounds[i] = editRaw(raw, func(v *jsonorder.Value) {
			resolver := &jsonorder.Value{Kind: jsonorder.Object}
			resolver.Set("server", jsonorder.String(server))
			if strategy != "" {
				resolver.Set("strategy", jsonorder.String(strategy))
			}
			v.Set("domain_resolver", resolver)
			v.Reorder(outboundOrder...)
		})
	}
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
	return []json.RawMessage{json.RawMessage(`{"type":"direct","tag":"` + DirectTag + `"}`)}
}

func normalizeOutbounds(outbounds []json.RawMessage) []json.RawMessage {
	hasDirect := false
	for i, raw := range outbounds {
		var header OutboundHeader
		if json.Unmarshal(raw, &header) != nil {
			continue
		}
		if header.Tag == LegacyDirectTag {
			outbounds[i] = editRaw(raw, func(v *jsonorder.Value) { v.Set("tag", jsonorder.String(DirectTag)) })
			header.Tag = DirectTag
		}
		if header.Tag == DirectTag {
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
		var item struct {
			Detour string `json:"download_detour"`
		}
		if json.Unmarshal(raw, &item) == nil && item.Detour == LegacyDirectTag {
			ruleSets[i] = editRaw(raw, func(v *jsonorder.Value) { v.Set("download_detour", jsonorder.String(DirectTag)) })
		}
	}
	return ruleSets
}

func ensureBaseRouteRules(rules []RouteRule) []RouteRule {
	hasSniff := false
	hasHijackDNS := false
	hasPrivateDirect := false

	for i := range rules {
		rules[i].Outbound = DirectOutbound(rules[i].Outbound)
		if rules[i].Action == "sniff" {
			hasSniff = true
		}
		if rules[i].Action == "hijack-dns" && rules[i].Protocol == "dns" {
			hasHijackDNS = true
		}
		if rules[i].Action == "route" && rules[i].IPIsPrivate && rules[i].Outbound == DirectTag {
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
			Outbound:    DirectTag,
			IPIsPrivate: true,
		})
	}
	if len(base) == 0 {
		return rules
	}
	return append(base, rules...)
}
