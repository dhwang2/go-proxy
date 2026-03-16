package store

import "strings"

// UserManagement is the top-level structure for user-management.json.
type UserManagement struct {
	Schema   int                      `json:"schema"`
	Disabled map[string]DisabledEntry `json:"disabled,omitempty"`
	Expiry   map[string]string        `json:"expiry,omitempty"`
	Route    map[string][]string      `json:"route,omitempty"`
	Template map[string]string        `json:"template,omitempty"`
	Name     map[string]string        `json:"name,omitempty"`
	Groups   map[string][]string      `json:"groups,omitempty"`
}

// DisabledEntry records a deactivated user with protocol context.
type DisabledEntry struct {
	Proto string       `json:"proto"`
	Tag   string       `json:"tag"`
	User  DisabledUser `json:"user"`
}

// DisabledUser holds the user object that was disabled.
type DisabledUser struct {
	Name     string `json:"name,omitempty"`
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Flow     string `json:"flow,omitempty"`
}

// UserKey builds the canonical user key: "proto|tag|user_id".
func UserKey(proto, tag, userID string) string {
	return proto + "|" + tag + "|" + userID
}

// ParseUserKey splits a user key into proto, tag, and userID.
func ParseUserKey(key string) (proto, tag, userID string) {
	first := strings.Index(key, "|")
	if first < 0 {
		return key, "", ""
	}
	second := strings.Index(key[first+1:], "|")
	if second < 0 {
		return key[:first], key[first+1:], ""
	}
	second += first + 1
	return key[:first], key[first+1 : second], key[second+1:]
}

// NewUserManagement returns an initialized empty UserManagement.
func NewUserManagement() *UserManagement {
	return &UserManagement{
		Schema:   3,
		Disabled: make(map[string]DisabledEntry),
		Expiry:   make(map[string]string),
		Route:    make(map[string][]string),
		Template: make(map[string]string),
		Name:     make(map[string]string),
		Groups:   make(map[string][]string),
	}
}
