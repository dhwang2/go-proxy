package store

import "encoding/json"

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
	Level     string `json:"level,omitempty"`
	Timestamp bool   `json:"timestamp,omitempty"`
}

// DNSConfig holds sing-box DNS configuration.
type DNSConfig struct {
	Servers []json.RawMessage `json:"servers,omitempty"`
	Rules   []DNSRule         `json:"rules,omitempty"`
	Final   string            `json:"final,omitempty"`
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
	Rules                 []RouteRule       `json:"rules,omitempty"`
	RuleSet               []json.RawMessage `json:"rule_set,omitempty"`
}

// RouteRule is a sing-box route rule.
type RouteRule struct {
	Action        string   `json:"action,omitempty"`
	Outbound      string   `json:"outbound,omitempty"`
	AuthUser      []string `json:"auth_user,omitempty"`
	Inbound       []string `json:"inbound,omitempty"`
	RuleSet       []string `json:"rule_set,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
}
