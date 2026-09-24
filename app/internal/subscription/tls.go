package subscription

import (
	"go-proxy/internal/crypto"
	"go-proxy/internal/store"
)

// The client side of a node's TLS settings, shared by every export that has to
// describe the handshake rather than just name a port.
type clientTLS struct {
	Enabled    bool           `json:"enabled"`
	ServerName string         `json:"server_name"`
	ALPN       []string       `json:"alpn,omitempty"`
	Reality    *clientReality `json:"reality,omitempty"`
	UTLS       *clientUTLS    `json:"utls,omitempty"`
}

type clientReality struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id,omitempty"`
}

type clientUTLS struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint"`
}

// buildClientTLS derives what a client needs from what the server holds. The
// Reality private key never leaves the server: the client is given the public
// key computed from it.
func buildClientTLS(tls *store.TLSConfig) (*clientTLS, error) {
	t := &clientTLS{Enabled: true, ServerName: tls.ServerName, ALPN: tls.ALPN}
	if tls.Reality != nil && tls.Reality.Enabled {
		publicKey, err := crypto.RealityPublicKey(tls.Reality.PrivateKey)
		if err != nil {
			return nil, err
		}
		t.Reality = &clientReality{Enabled: true, PublicKey: publicKey}
		if len(tls.Reality.ShortID) > 0 {
			t.Reality.ShortID = tls.Reality.ShortID[0]
		}
		t.UTLS = &clientUTLS{Enabled: true, Fingerprint: "chrome"}
	}
	return t, nil
}
