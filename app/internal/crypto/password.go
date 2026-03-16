package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GeneratePassword generates a cryptographically random alphanumeric password.
func GeneratePassword(length int) (string, error) {
	if length <= 0 {
		length = 16
	}
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphanumeric))))
		if err != nil {
			return "", fmt.Errorf("generate password: %w", err)
		}
		result[i] = alphanumeric[n.Int64()]
	}
	return string(result), nil
}
