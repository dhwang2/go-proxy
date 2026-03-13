package derived

import (
	"sort"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

type Membership struct {
	UserName string
	Protocol string
	Tag      string
	UserID   string
	State    string
}

func ComputeMemberships(config *store.SingboxConfig, meta *store.UserMeta) []Membership {
	if config == nil || meta == nil {
		return nil
	}
	rows := make([]Membership, 0, len(config.Inbounds))
	for _, inbound := range config.Inbounds {
		proto := normalizeProto(inbound.Type)
		for _, user := range inbound.Users {
			userID := strings.TrimSpace(user.Key())
			if userID == "" {
				continue
			}
			key := storeUserMetaKey(proto, inbound.Tag, userID)
			name := strings.TrimSpace(meta.Name[key])
			if name == "" {
				name = strings.TrimSpace(user.Name)
			}
			if name == "" {
				name = "user"
			}
			rows = append(rows, Membership{
				UserName: name,
				Protocol: proto,
				Tag:      inbound.Tag,
				UserID:   userID,
				State:    "active",
			})
		}
	}

	for key, disabled := range meta.Disabled {
		name := strings.TrimSpace(disabled.User.Name)
		if name == "" {
			name = strings.TrimSpace(meta.Name[key])
		}
		if name == "" {
			name = "user"
		}
		rows = append(rows, Membership{
			UserName: name,
			Protocol: normalizeProto(disabled.Proto),
			Tag:      disabled.Tag,
			UserID:   key,
			State:    "disabled",
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UserName != rows[j].UserName {
			return rows[i].UserName < rows[j].UserName
		}
		if rows[i].State != rows[j].State {
			return rows[i].State < rows[j].State
		}
		if rows[i].Protocol != rows[j].Protocol {
			return rows[i].Protocol < rows[j].Protocol
		}
		if rows[i].Tag != rows[j].Tag {
			return rows[i].Tag < rows[j].Tag
		}
		return rows[i].UserID < rows[j].UserID
	})
	return rows
}

func storeUserMetaKey(proto, tag, userID string) string {
	return strings.ToLower(strings.TrimSpace(proto)) + "|" + strings.TrimSpace(tag) + "|" + strings.TrimSpace(userID)
}

func normalizeProto(proto string) string {
	s := strings.ToLower(strings.TrimSpace(proto))
	if s == "shadowsocks" {
		return "ss"
	}
	return s
}
