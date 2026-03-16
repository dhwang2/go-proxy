package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// RealityKeypair holds a Reality X25519 key pair in base64 raw-URL encoding.
type RealityKeypair struct {
	PrivateKey string
	PublicKey  string
}

// GenerateRealityKeypair generates an X25519 key pair for sing-box Reality TLS.
func GenerateRealityKeypair() (*RealityKeypair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate x25519 key: %w", err)
	}
	return &RealityKeypair{
		PrivateKey: base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		PublicKey:  base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
	}, nil
}

// GenerateShortID generates a random hex short ID for Reality (8 hex chars).
func GenerateShortID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate short id: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}
