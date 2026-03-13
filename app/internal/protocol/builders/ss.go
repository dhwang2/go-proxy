package builders

import (
	"encoding/json"

	"github.com/dhwang2/go-proxy/internal/store"
)

// SSSpec defines the parameters for building a Shadowsocks inbound.
type SSSpec struct {
	Tag       string
	Port      int
	Method    string
	ServerPSK string // server-level password/PSK for 2022-blake3 methods
	Users     []store.User
}

// BuildSSInbound creates a store.Inbound configured for the Shadowsocks protocol.
// The method and server PSK are set at the inbound level. Users have per-user passwords.
func BuildSSInbound(spec SSSpec) (store.Inbound, error) {
	raw := map[string]json.RawMessage{}
	setRaw(raw, "listen", "::")

	method := spec.Method
	if method == "" {
		method = "2022-blake3-aes-128-gcm"
	}
	setRaw(raw, "method", method)

	if spec.ServerPSK != "" {
		setRaw(raw, "password", spec.ServerPSK)
	}

	return store.Inbound{
		Type:       "shadowsocks",
		Tag:        spec.Tag,
		ListenPort: spec.Port,
		Users:      spec.Users,
		Raw:        raw,
	}, nil
}
