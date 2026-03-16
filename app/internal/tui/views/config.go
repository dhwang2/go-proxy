package views

import (
	"encoding/json"

	"go-proxy/internal/store"
)

// RenderConfig returns a formatted JSON view of the sing-box config.
func RenderConfig(s *store.Store) string {
	data, err := json.MarshalIndent(s.SingBox, "", "  ")
	if err != nil {
		return "Error rendering config: " + err.Error()
	}
	return string(data)
}
