package crypto

import (
	"crypto/ecdh"
	"encoding/base64"
)

// DeriveRealityPublicKey derives a Reality X25519 public key from a private key.
// sing-box uses URL-safe base64 encoding (RFC 4648 §5) without padding for Reality keys.
func DeriveRealityPublicKey(privateKeyBase64 string) (string, error) {
	privBytes, err := base64.RawURLEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return "", err
	}
	curve := ecdh.X25519()
	priv, err := curve.NewPrivateKey(privBytes)
	if err != nil {
		return "", err
	}
	pub := priv.PublicKey().Bytes()
	return base64.RawURLEncoding.EncodeToString(pub), nil
}
