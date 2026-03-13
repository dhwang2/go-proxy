package builders

import (
	"fmt"

	"github.com/dhwang2/go-proxy/internal/store"
)

// SnellSpec defines the parameters for building a snell configuration.
type SnellSpec struct {
	Port int
	PSK  string
}

// BuildSnellConfig creates a SnellConfig with listen, psk, and obfs settings.
// Returns an error if port or PSK is missing.
func BuildSnellConfig(spec SnellSpec) (*store.SnellConfig, error) {
	if spec.Port <= 0 {
		return nil, fmt.Errorf("snell requires a valid port")
	}
	if spec.PSK == "" {
		return nil, fmt.Errorf("snell requires a PSK")
	}

	return &store.SnellConfig{
		Values: map[string]string{
			"listen": fmt.Sprintf("0.0.0.0:%d", spec.Port),
			"psk":    spec.PSK,
			"obfs":   "off",
		},
	}, nil
}
