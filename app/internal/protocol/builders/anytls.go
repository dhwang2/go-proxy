package builders

import (
	"encoding/json"
	"fmt"

	"github.com/dhwang2/go-proxy/internal/store"
)

// AnytlsSpec defines the parameters for building an anytls inbound.
type AnytlsSpec struct {
	Tag   string
	Port  int
	Users []store.User
	TLS   json.RawMessage
}

// BuildAnytlsInbound creates a store.Inbound configured for the anytls protocol.
// TLS is required. Users should have the Password field set.
func BuildAnytlsInbound(spec AnytlsSpec) (store.Inbound, error) {
	if len(spec.TLS) == 0 {
		return store.Inbound{}, fmt.Errorf("anytls requires TLS configuration")
	}

	raw := map[string]json.RawMessage{}
	setRaw(raw, "listen", "::")

	var tlsMap map[string]json.RawMessage
	if err := json.Unmarshal(spec.TLS, &tlsMap); err != nil {
		tlsMap = map[string]json.RawMessage{}
	}
	setRaw(tlsMap, "enabled", true)
	b, _ := json.Marshal(tlsMap)
	raw["tls"] = b

	return store.Inbound{
		Type:       "anytls",
		Tag:        spec.Tag,
		ListenPort: spec.Port,
		Users:      spec.Users,
		Raw:        raw,
	}, nil
}
