package routing

import (
	"encoding/json"
	"fmt"

	"go-proxy/internal/store"
)

// ChainOutbound represents a SOCKS chain proxy outbound.
type ChainOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Version    string `json:"version,omitempty"`
}

// SetChain adds or updates a SOCKS chain proxy outbound.
func SetChain(s *store.Store, tag, server string, port int) error {
	if tag == "" || server == "" || port <= 0 {
		return fmt.Errorf("invalid chain parameters")
	}

	outbound := ChainOutbound{
		Type:       "socks",
		Tag:        tag,
		Server:     server,
		ServerPort: port,
		Version:    "5",
	}
	raw, err := json.Marshal(outbound)
	if err != nil {
		return fmt.Errorf("marshal chain outbound: %w", err)
	}

	// Replace existing or append.
	replaced := false
	for i, ob := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(ob)
		if h.Tag == tag {
			s.SingBox.Outbounds[i] = raw
			replaced = true
			break
		}
	}
	if !replaced {
		s.SingBox.Outbounds = append(s.SingBox.Outbounds, raw)
	}

	s.MarkDirty(store.FileSingBox)
	return nil
}

// RemoveChain removes a chain proxy outbound by tag.
func RemoveChain(s *store.Store, tag string) error {
	for i, ob := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(ob)
		if h.Tag == tag {
			s.SingBox.Outbounds = append(s.SingBox.Outbounds[:i], s.SingBox.Outbounds[i+1:]...)
			s.MarkDirty(store.FileSingBox)
			return nil
		}
	}
	return fmt.Errorf("outbound %q not found", tag)
}
