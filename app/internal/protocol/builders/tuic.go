package builders

import (
	"encoding/json"
	"fmt"

	"github.com/dhwang2/go-proxy/internal/store"
)

// TuicSpec defines the parameters for building a tuic inbound.
type TuicSpec struct {
	Tag   string
	Port  int
	Users []store.User
	TLS   json.RawMessage
}

// BuildTuicInbound creates a store.Inbound configured for the tuic protocol.
// Sets congestion_control="bbr" and requires TLS. Users should have uuid+password.
func BuildTuicInbound(spec TuicSpec) (store.Inbound, error) {
	if len(spec.TLS) == 0 {
		return store.Inbound{}, fmt.Errorf("tuic requires TLS configuration")
	}

	raw := map[string]json.RawMessage{}
	setRaw(raw, "listen", "::")
	setRaw(raw, "congestion_control", "bbr")

	var tlsMap map[string]json.RawMessage
	if err := json.Unmarshal(spec.TLS, &tlsMap); err != nil {
		tlsMap = map[string]json.RawMessage{}
	}
	setRaw(tlsMap, "enabled", true)
	b, _ := json.Marshal(tlsMap)
	raw["tls"] = b

	return store.Inbound{
		Type:       "tuic",
		Tag:        spec.Tag,
		ListenPort: spec.Port,
		Users:      spec.Users,
		Raw:        raw,
	}, nil
}
