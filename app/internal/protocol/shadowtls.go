package protocol

import (
	"fmt"
)

// ShadowTLSWrapSpec defines a shadow-tls v3 wrapper around an inner protocol.
type ShadowTLSWrapSpec struct {
	InnerTag   string // tag of the wrapped inbound
	InnerPort  int    // listen port of the inner protocol
	ListenPort int    // external shadow-tls listen port
	Password   string // shadow-tls handshake password
	SNI        string // TLS handshake SNI (e.g., "www.microsoft.com")
}

// BuildShadowTLSWrap creates a shadow-tls v3 wrapper inbound configuration.
// Returns a map with keys for shadow-tls CLI arguments:
//
//	"listen"   - the address to listen on (e.g. "0.0.0.0:1234")
//	"server"   - the inner protocol address (e.g. "127.0.0.1:5678")
//	"password" - the shadow-tls handshake password
//	"tls.sni"  - the TLS handshake SNI
func BuildShadowTLSWrap(spec ShadowTLSWrapSpec) (map[string]string, error) {
	if spec.ListenPort <= 0 {
		return nil, fmt.Errorf("shadow-tls requires a valid listen port")
	}
	if spec.InnerPort <= 0 {
		return nil, fmt.Errorf("shadow-tls requires a valid inner port")
	}
	if spec.Password == "" {
		return nil, fmt.Errorf("shadow-tls requires a password")
	}

	sni := spec.SNI
	if sni == "" {
		sni = "www.microsoft.com"
	}

	return map[string]string{
		"listen":   fmt.Sprintf("0.0.0.0:%d", spec.ListenPort),
		"server":   fmt.Sprintf("127.0.0.1:%d", spec.InnerPort),
		"password": spec.Password,
		"tls.sni":  sni,
	}, nil
}
