package protocol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

// EnsureTLS validates that a TLS configuration exists for the given protocol.
// If sni is provided, creates a basic TLS config with that server_name.
// Otherwise, copies TLS config from an existing inbound of the same type,
// falling back to any inbound with TLS.
func EnsureTLS(cfg *store.SingboxConfig, proto, sni string) (json.RawMessage, error) {
	return resolveTLSRaw(cfg, proto, sni)
}

// resolveTLSRaw resolves TLS configuration for a protocol installation.
// It first checks for an explicit SNI, then looks for existing TLS configs
// on same-type inbounds, and finally falls back to any inbound with TLS.
func resolveTLSRaw(cfg *store.SingboxConfig, proto, sni string) (json.RawMessage, error) {
	sni = strings.TrimSpace(sni)
	if sni != "" {
		b, _ := json.Marshal(map[string]any{"server_name": sni})
		return b, nil
	}

	// Try to find TLS config from an existing inbound of the same protocol type.
	for _, in := range cfg.Inbounds {
		if normalizeProtocolType(in.Type) != proto {
			continue
		}
		if in.Raw == nil || len(in.Raw["tls"]) == 0 {
			continue
		}
		return append(json.RawMessage(nil), in.Raw["tls"]...), nil
	}

	// Fall back to any inbound with TLS.
	for _, in := range cfg.Inbounds {
		if in.Raw == nil || len(in.Raw["tls"]) == 0 {
			continue
		}
		return append(json.RawMessage(nil), in.Raw["tls"]...), nil
	}

	return nil, fmt.Errorf("protocol %s requires tls template; please pass --sni or install after a tls inbound exists", proto)
}
