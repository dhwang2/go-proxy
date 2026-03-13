package builders

import (
	"encoding/json"

	"github.com/dhwang2/go-proxy/internal/store"
)

// VlessSpec defines the parameters for building a vless inbound.
type VlessSpec struct {
	Tag   string
	Port  int
	Users []store.User
	TLS   json.RawMessage // optional TLS config
}

// BuildVlessInbound creates a store.Inbound configured for the vless protocol.
// It sets listen="::", flow="xtls-rprx-vision" on each user, and optionally
// attaches TLS configuration with enabled=true.
func BuildVlessInbound(spec VlessSpec) (store.Inbound, error) {
	raw := map[string]json.RawMessage{}
	setRaw(raw, "listen", "::")

	users := make([]store.User, len(spec.Users))
	for i, u := range spec.Users {
		users[i] = u
		if users[i].Raw == nil {
			users[i].Raw = map[string]json.RawMessage{}
		}
		setRaw(users[i].Raw, "flow", "xtls-rprx-vision")
	}

	if len(spec.TLS) > 0 {
		// Merge enabled=true into the provided TLS config.
		var tlsMap map[string]json.RawMessage
		if err := json.Unmarshal(spec.TLS, &tlsMap); err != nil {
			tlsMap = map[string]json.RawMessage{}
		}
		setRaw(tlsMap, "enabled", true)
		b, _ := json.Marshal(tlsMap)
		raw["tls"] = b
	}

	return store.Inbound{
		Type:       "vless",
		Tag:        spec.Tag,
		ListenPort: spec.Port,
		Users:      users,
		Raw:        raw,
	}, nil
}
