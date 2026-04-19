package routing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go-proxy/internal/store"
)

const (
	// ChainTagPrefix is the tag prefix for res-socks chain proxy outbounds.
	// Matches shell-proxy: res-socks1, res-socks2, etc.
	ChainTagPrefix = "res-socks"

	// ChainDNSTagPrefix is the DNS server tag prefix for chain proxy outbounds.
	ChainDNSTagPrefix = "res-proxy"
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

// AddChain adds a new SOCKS chain proxy outbound with auto-generated tag.
// Input format matches shell-proxy: server:port:username:password.
func AddChain(s *store.Store, server string, port int, username, password string) (string, error) {
	if server == "" || port <= 0 {
		return "", fmt.Errorf("invalid chain parameters")
	}

	tag := nextChainTag(s)
	outbound := ChainOutbound{
		Type:       "socks",
		Tag:        tag,
		Server:     server,
		ServerPort: port,
		Username:   username,
		Password:   password,
		Version:    "5",
	}
	raw, err := json.Marshal(outbound)
	if err != nil {
		return "", fmt.Errorf("marshal chain outbound: %w", err)
	}

	s.SingBox.Outbounds = append(s.SingBox.Outbounds, raw)
	s.MarkDirty(store.FileSingBox)
	return tag, nil
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

// ListChains returns all chain proxy outbounds from the store.
func ListChains(s *store.Store) []ChainOutbound {
	var chains []ChainOutbound
	for _, raw := range s.SingBox.Outbounds {
		h, err := store.ParseOutboundHeader(raw)
		if err != nil || !IsChainTag(h.Tag) {
			continue
		}
		var co ChainOutbound
		if err := json.Unmarshal(raw, &co); err == nil {
			chains = append(chains, co)
		}
	}
	return chains
}

// IsChainTag returns whether a tag belongs to a chain proxy outbound.
func IsChainTag(tag string) bool {
	if tag == ChainTagPrefix {
		return true
	}
	suffix := strings.TrimPrefix(tag, ChainTagPrefix)
	if suffix == tag {
		return false
	}
	_, err := strconv.Atoi(suffix)
	return err == nil
}

// ChainDNSTag returns the DNS server tag for a chain proxy outbound tag.
func ChainDNSTag(outboundTag string) string {
	suffix := strings.TrimPrefix(outboundTag, ChainTagPrefix)
	if suffix == outboundTag {
		return ChainDNSTagPrefix
	}
	return ChainDNSTagPrefix + suffix
}

// nextChainTag finds the next available res-socks{N} tag.
func nextChainTag(s *store.Store) string {
	used := make(map[int]bool)
	for _, raw := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(raw)
		if !strings.HasPrefix(h.Tag, ChainTagPrefix) {
			continue
		}
		suffix := strings.TrimPrefix(h.Tag, ChainTagPrefix)
		if n, err := strconv.Atoi(suffix); err == nil {
			used[n] = true
		}
	}
	for i := 1; ; i++ {
		if !used[i] {
			return ChainTagPrefix + strconv.Itoa(i)
		}
	}
}
