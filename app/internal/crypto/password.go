package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"
	"strings"
)

const passwordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~@#%+="

func NewPassword(length int) (string, error) {
	if length <= 0 {
		length = 20
	}
	out := make([]byte, length)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// NewSSKey generates a base64-encoded random key for Shadowsocks 2022 methods.
// Key length depends on the cipher: AES-128 = 16 bytes, AES-256/ChaCha20 = 32 bytes.
func NewSSKey(method string) (string, error) {
	n := 32
	if strings.Contains(method, "aes-128") {
		n = 16
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// IsSSAEAD2022 reports whether the given method is a Shadowsocks AEAD 2022 cipher
// that requires base64-encoded keys.
func IsSSAEAD2022(method string) bool {
	return strings.HasPrefix(method, "2022-blake3-")
}
