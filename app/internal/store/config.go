package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SingboxConfig holds the primary sing-box state.
// Unknown top-level fields are preserved in Extra for forward compatibility.
type SingboxConfig struct {
	Inbounds  []Inbound                  `json:"inbounds"`
	Outbounds []Outbound                 `json:"outbounds"`
	Route     Route                      `json:"route"`
	DNS       DNS                        `json:"dns"`
	Extra     map[string]json.RawMessage `json:"-"`
}

func (c *SingboxConfig) UnmarshalJSON(data []byte) error {
	type alias SingboxConfig
	var known alias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	delete(raw, "inbounds")
	delete(raw, "outbounds")
	delete(raw, "route")
	delete(raw, "dns")
	*c = SingboxConfig(known)
	if len(raw) > 0 {
		c.Extra = raw
	}
	return nil
}

func (c SingboxConfig) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range c.Extra {
		m[k] = v
	}
	if b, err := json.Marshal(c.Inbounds); err == nil {
		m["inbounds"] = b
	} else {
		return nil, err
	}
	if b, err := json.Marshal(c.Outbounds); err == nil {
		m["outbounds"] = b
	} else {
		return nil, err
	}
	if b, err := json.Marshal(c.Route); err == nil {
		m["route"] = b
	} else {
		return nil, err
	}
	if b, err := json.Marshal(c.DNS); err == nil {
		m["dns"] = b
	} else {
		return nil, err
	}
	return json.Marshal(m)
}

type Inbound struct {
	Type       string                     `json:"type"`
	Tag        string                     `json:"tag"`
	ListenPort int                        `json:"listen_port,omitempty"`
	Users      []User                     `json:"users,omitempty"`
	Raw        map[string]json.RawMessage `json:"-"`
}

func (i *Inbound) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	i.Raw = raw
	_ = json.Unmarshal(raw["type"], &i.Type)
	_ = json.Unmarshal(raw["tag"], &i.Tag)
	_ = json.Unmarshal(raw["listen_port"], &i.ListenPort)

	usersField, ok := raw["users"]
	if !ok {
		return i.readLegacySingleUser(raw)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(usersField, &arr); err == nil {
		i.Users = make([]User, 0, len(arr))
		for _, item := range arr {
			var u User
			if err := json.Unmarshal(item, &u); err == nil {
				i.Users = append(i.Users, u)
			}
		}
		return nil
	}
	var one User
	if err := json.Unmarshal(usersField, &one); err == nil {
		i.Users = []User{one}
		return nil
	}
	return i.readLegacySingleUser(raw)
}

func (i *Inbound) readLegacySingleUser(raw map[string]json.RawMessage) error {
	var legacy User
	if _, ok := raw["uuid"]; ok {
		_ = json.Unmarshal(raw["uuid"], &legacy.UUID)
		_ = json.Unmarshal(raw["id"], &legacy.ID)
		_ = json.Unmarshal(raw["name"], &legacy.Name)
		_ = json.Unmarshal(raw["password"], &legacy.Password)
		if legacy.ID == "" {
			legacy.ID = legacy.UUID
		}
		i.Users = []User{legacy}
		return nil
	}
	if _, ok := raw["password"]; ok {
		_ = json.Unmarshal(raw["password"], &legacy.Password)
		_ = json.Unmarshal(raw["name"], &legacy.Name)
		_ = json.Unmarshal(raw["method"], &legacy.Method)
		if legacy.Password != "" {
			i.Users = []User{legacy}
		}
	}
	return nil
}

func (i Inbound) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range i.Raw {
		m[k] = v
	}
	writeJSONField(m, "type", i.Type)
	writeJSONField(m, "tag", i.Tag)
	if i.ListenPort > 0 {
		writeJSONField(m, "listen_port", i.ListenPort)
	} else {
		delete(m, "listen_port")
	}
	if len(i.Users) > 0 {
		writeJSONField(m, "users", i.Users)
	} else {
		delete(m, "users")
	}
	return json.Marshal(m)
}

type Outbound struct {
	Type string                     `json:"type"`
	Tag  string                     `json:"tag"`
	Raw  map[string]json.RawMessage `json:"-"`
}

func (o *Outbound) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &o.Raw); err != nil {
		return err
	}
	_ = json.Unmarshal(o.Raw["type"], &o.Type)
	_ = json.Unmarshal(o.Raw["tag"], &o.Tag)
	return nil
}

func (o Outbound) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range o.Raw {
		m[k] = v
	}
	writeJSONField(m, "type", o.Type)
	writeJSONField(m, "tag", o.Tag)
	return json.Marshal(m)
}

type User struct {
	Name     string                     `json:"name,omitempty"`
	UUID     string                     `json:"uuid,omitempty"`
	ID       string                     `json:"id,omitempty"`
	Password string                     `json:"password,omitempty"`
	Method   string                     `json:"method,omitempty"`
	Raw      map[string]json.RawMessage `json:"-"`
}

func (u *User) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &u.Raw); err != nil {
		return err
	}
	_ = json.Unmarshal(u.Raw["name"], &u.Name)
	_ = json.Unmarshal(u.Raw["uuid"], &u.UUID)
	_ = json.Unmarshal(u.Raw["id"], &u.ID)
	_ = json.Unmarshal(u.Raw["password"], &u.Password)
	_ = json.Unmarshal(u.Raw["method"], &u.Method)
	if u.ID == "" {
		u.ID = u.UUID
	}
	return nil
}

func (u User) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range u.Raw {
		m[k] = v
	}
	writeJSONField(m, "name", u.Name)
	if u.UUID != "" {
		writeJSONField(m, "uuid", u.UUID)
	}
	// ID and Method are internal go-proxy fields; do not write to sing-box config.
	// sing-box 1.13+ rejects unknown fields. ID is redundant with UUID/Password.
	// Method belongs at inbound level (not per-user) for shadowsocks.
	delete(m, "id")
	delete(m, "method")
	if u.Password != "" {
		writeJSONField(m, "password", u.Password)
	}
	return json.Marshal(m)
}

func (u User) Key() string {
	switch {
	case u.ID != "":
		return u.ID
	case u.UUID != "":
		return u.UUID
	default:
		return u.Password
	}
}

type Route struct {
	Rules []RouteRule                `json:"rules,omitempty"`
	Raw   map[string]json.RawMessage `json:"-"`
}

func (r *Route) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if err := json.Unmarshal(data, &r.Raw); err != nil {
		return err
	}
	_ = json.Unmarshal(r.Raw["rules"], &r.Rules)
	return nil
}

func (r Route) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range r.Raw {
		m[k] = v
	}
	if len(r.Rules) > 0 {
		writeJSONField(m, "rules", r.Rules)
	} else {
		delete(m, "rules")
	}
	return json.Marshal(m)
}

type RouteRule struct {
	Action        string                     `json:"action,omitempty"`
	Outbound      string                     `json:"outbound,omitempty"`
	AuthUser      []string                   `json:"auth_user,omitempty"`
	RuleSet       []string                   `json:"rule_set,omitempty"`
	Domain        []string                   `json:"domain,omitempty"`
	DomainSuffix  []string                   `json:"domain_suffix,omitempty"`
	DomainKeyword []string                   `json:"domain_keyword,omitempty"`
	DomainRegex   []string                   `json:"domain_regex,omitempty"`
	Raw           map[string]json.RawMessage `json:"-"`
}

func (r *RouteRule) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &r.Raw); err != nil {
		return err
	}
	_ = json.Unmarshal(r.Raw["action"], &r.Action)
	_ = json.Unmarshal(r.Raw["outbound"], &r.Outbound)
	r.AuthUser = readStringList(r.Raw["auth_user"])
	r.RuleSet = readStringList(r.Raw["rule_set"])
	r.Domain = readStringList(r.Raw["domain"])
	r.DomainSuffix = readStringList(r.Raw["domain_suffix"])
	r.DomainKeyword = readStringList(r.Raw["domain_keyword"])
	r.DomainRegex = readStringList(r.Raw["domain_regex"])
	return nil
}

func (r RouteRule) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range r.Raw {
		m[k] = v
	}
	writeJSONField(m, "action", r.Action)
	writeJSONField(m, "outbound", r.Outbound)
	writeStringListField(m, "auth_user", r.AuthUser)
	writeStringListField(m, "rule_set", r.RuleSet)
	writeStringListField(m, "domain", r.Domain)
	writeStringListField(m, "domain_suffix", r.DomainSuffix)
	writeStringListField(m, "domain_keyword", r.DomainKeyword)
	writeStringListField(m, "domain_regex", r.DomainRegex)
	return json.Marshal(m)
}

type DNS struct {
	Rules []DNSRule                  `json:"rules,omitempty"`
	Raw   map[string]json.RawMessage `json:"-"`
}

func (d *DNS) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if err := json.Unmarshal(data, &d.Raw); err != nil {
		return err
	}
	_ = json.Unmarshal(d.Raw["rules"], &d.Rules)
	return nil
}

func (d DNS) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range d.Raw {
		m[k] = v
	}
	if len(d.Rules) > 0 {
		writeJSONField(m, "rules", d.Rules)
	} else {
		delete(m, "rules")
	}
	return json.Marshal(m)
}

type DNSRule struct {
	Action   string                     `json:"action,omitempty"`
	Server   string                     `json:"server,omitempty"`
	AuthUser []string                   `json:"auth_user,omitempty"`
	RuleSet  []string                   `json:"rule_set,omitempty"`
	Raw      map[string]json.RawMessage `json:"-"`
}

func (d *DNSRule) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &d.Raw); err != nil {
		return err
	}
	_ = json.Unmarshal(d.Raw["action"], &d.Action)
	_ = json.Unmarshal(d.Raw["server"], &d.Server)
	d.AuthUser = readStringList(d.Raw["auth_user"])
	d.RuleSet = readStringList(d.Raw["rule_set"])
	return nil
}

func (d DNSRule) MarshalJSON() ([]byte, error) {
	m := map[string]json.RawMessage{}
	for k, v := range d.Raw {
		m[k] = v
	}
	writeJSONField(m, "action", d.Action)
	writeJSONField(m, "server", d.Server)
	writeStringListField(m, "auth_user", d.AuthUser)
	writeStringListField(m, "rule_set", d.RuleSet)
	return json.Marshal(m)
}

func writeJSONField(m map[string]json.RawMessage, key string, v any) {
	if key == "" {
		return
	}
	switch x := v.(type) {
	case string:
		if x == "" {
			delete(m, key)
			return
		}
	case []string:
		if len(x) == 0 {
			delete(m, key)
			return
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	m[key] = b
}

func writeStringListField(m map[string]json.RawMessage, key string, items []string) {
	items = uniqueStrings(items)
	if len(items) == 0 {
		delete(m, key)
		return
	}
	writeJSONField(m, key, items)
}

func readStringList(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return uniqueStrings(arr)
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		if one == "" {
			return nil
		}
		return []string{one}
	}
	return nil
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizeProto(proto string) string {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "shadowsocks":
		return "ss"
	default:
		return strings.ToLower(strings.TrimSpace(proto))
	}
}

func userMetaKey(proto, tag, userID string) string {
	return fmt.Sprintf("%s|%s|%s", normalizeProto(proto), strings.TrimSpace(tag), strings.TrimSpace(userID))
}
