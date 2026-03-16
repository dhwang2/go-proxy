package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// DefaultSSMethod is the default Shadowsocks 2022 encryption method.
const DefaultSSMethod = "2022-blake3-aes-256-gcm"

// GenerateSSKey generates a random base64-encoded key for Shadowsocks 2022.
// keySize is the number of random bytes: 16 for AES-128, 32 for AES-256.
func GenerateSSKey(keySize int) (string, error) {
	if keySize != 16 && keySize != 32 {
		return "", fmt.Errorf("invalid key size %d: must be 16 or 32", keySize)
	}
	b := make([]byte, keySize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate ss key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SSKeySize returns the required key size for a Shadowsocks 2022 method.
func SSKeySize(method string) int {
	switch method {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	default:
		return 32
	}
}
