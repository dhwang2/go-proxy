package store

import (
	"encoding/json"
	"strings"
	"time"
)

type UserMeta struct {
	Schema   int                        `json:"schema"`
	Disabled map[string]DisabledUser    `json:"disabled"`
	Expiry   map[string]string          `json:"expiry"`
	Route    map[string]json.RawMessage `json:"route"`
	Template map[string]string          `json:"template"`
	Name     map[string]string          `json:"name"`
	Groups   map[string]Group           `json:"groups"`
	Extra    map[string]json.RawMessage `json:"-"`
}

type Group struct {
	CreatedAt string `json:"created_at,omitempty"`
}

type DisabledUser struct {
	Proto string `json:"proto,omitempty"`
	Tag   string `json:"tag,omitempty"`
	User  User   `json:"user,omitempty"`
}

func DefaultUserMeta() *UserMeta {
	return &UserMeta{
		Schema:   3,
		Disabled: map[string]DisabledUser{},
		Expiry:   map[string]string{},
		Route:    map[string]json.RawMessage{},
		Template: map[string]string{},
		Name:     map[string]string{},
		Groups:   map[string]Group{},
	}
}

func (m *UserMeta) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*m = *DefaultUserMeta()
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	meta := DefaultUserMeta()
	_ = json.Unmarshal(raw["schema"], &meta.Schema)
	_ = json.Unmarshal(raw["disabled"], &meta.Disabled)
	_ = json.Unmarshal(raw["expiry"], &meta.Expiry)
	_ = json.Unmarshal(raw["route"], &meta.Route)
	_ = json.Unmarshal(raw["template"], &meta.Template)
	if err := unmarshalNameMap(raw["name"], &meta.Name); err != nil {
		return err
	}
	_ = json.Unmarshal(raw["groups"], &meta.Groups)
	if meta.Schema < 3 {
		meta.Schema = 3
	}
	delete(raw, "schema")
	delete(raw, "disabled")
	delete(raw, "expiry")
	delete(raw, "route")
	delete(raw, "template")
	delete(raw, "name")
	delete(raw, "groups")
	if len(raw) > 0 {
		meta.Extra = raw
	}
	*m = *meta
	m.ensureMaps()
	return nil
}

func (m UserMeta) MarshalJSON() ([]byte, error) {
	m.ensureMaps()
	out := map[string]json.RawMessage{}
	for k, v := range m.Extra {
		out[k] = v
	}
	writeJSONField(out, "schema", m.Schema)
	writeJSONField(out, "disabled", m.Disabled)
	writeJSONField(out, "expiry", m.Expiry)
	writeJSONField(out, "route", m.Route)
	writeJSONField(out, "template", m.Template)
	writeJSONField(out, "name", m.Name)
	writeJSONField(out, "groups", m.Groups)
	return json.Marshal(out)
}

func (m *UserMeta) EnsureDefaults() {
	if m.Schema < 3 {
		m.Schema = 3
	}
	m.ensureMaps()
}

func (m *UserMeta) ensureMaps() {
	if m.Disabled == nil {
		m.Disabled = map[string]DisabledUser{}
	}
	if m.Expiry == nil {
		m.Expiry = map[string]string{}
	}
	if m.Route == nil {
		m.Route = map[string]json.RawMessage{}
	}
	if m.Template == nil {
		m.Template = map[string]string{}
	}
	if m.Name == nil {
		m.Name = map[string]string{}
	}
	if m.Groups == nil {
		m.Groups = map[string]Group{}
	}
}

func (m *UserMeta) AddGroup(name string) {
	name = normalizeGroupName(name)
	if name == "" {
		return
	}
	m.ensureMaps()
	if _, ok := m.Groups[name]; ok {
		return
	}
	m.Groups[name] = Group{CreatedAt: utcNow()}
}

func (m *UserMeta) DeleteGroup(name string) {
	name = normalizeGroupName(name)
	if name == "" {
		return
	}
	m.ensureMaps()
	delete(m.Groups, name)
}

func (m *UserMeta) RenameGroup(oldName, newName string) {
	oldName = normalizeGroupName(oldName)
	newName = normalizeGroupName(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return
	}
	m.ensureMaps()
	group := m.Groups[oldName]
	if group.CreatedAt == "" {
		group.CreatedAt = utcNow()
	}
	m.Groups[newName] = group
	delete(m.Groups, oldName)
}

func normalizeGroupName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "\t", "-", "\n", "-")
	v = replacer.Replace(v)
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	v = strings.Trim(v, "-_.")
	if v == "" {
		return "user"
	}
	return v
}

func utcNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func unmarshalNameMap(raw json.RawMessage, target *map[string]string) error {
	if len(raw) == 0 {
		*target = map[string]string{}
		return nil
	}
	var strMap map[string]string
	if err := json.Unmarshal(raw, &strMap); err == nil {
		*target = strMap
		return nil
	}
	var objMap map[string]struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &objMap); err == nil {
		out := make(map[string]string, len(objMap))
		for k, v := range objMap {
			out[k] = v.Value
		}
		*target = out
		return nil
	}
	*target = map[string]string{}
	return nil
}
