package builders

import (
	"encoding/json"
	"fmt"

	"github.com/dhwang2/go-proxy/internal/store"
)

// TrojanSpec defines the parameters for building a trojan inbound.
type TrojanSpec struct {
	Tag   string
	Port  int
	Users []store.User
	TLS   json.RawMessage
}

// BuildTrojanInbound creates a store.Inbound configured for the trojan protocol.
// TLS is required for trojan; an error is returned if TLS is empty.
// Users are expected to have the Password field set.
func BuildTrojanInbound(spec TrojanSpec) (store.Inbound, error) {
	if len(spec.TLS) == 0 {
		return store.Inbound{}, fmt.Errorf("trojan requires TLS configuration")
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
		Type:       "trojan",
		Tag:        spec.Tag,
		ListenPort: spec.Port,
		Users:      spec.Users,
		Raw:        raw,
	}, nil
}
