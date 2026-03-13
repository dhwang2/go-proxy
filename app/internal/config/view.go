package config

import (
	"encoding/json"

	"github.com/dhwang2/go-proxy/internal/store"
)

func Pretty(cfg *store.SingboxConfig) (string, error) {
	if cfg == nil {
		return "{}", nil
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
