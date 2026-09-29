package store

import "strings"

// UserManagement is the top-level structure for user-management.json.
type UserManagement struct {
	Name   map[string]string   `json:"name,omitempty"`
	Groups map[string][]string `json:"groups,omitempty"`
	// ChainStrategy records, per chain tag, the address families that chain's
	// lookups ask for, decided when the chain was added or changed. A hostname
	// endpoint needs a lookup to decide it, which the compile path cannot make,
	// so the answer is kept here rather than in the sing-box configuration.
	ChainStrategy map[string]string `json:"chain_strategy,omitempty"`
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
		Name:   make(map[string]string),
		Groups: make(map[string][]string),
	}
}
