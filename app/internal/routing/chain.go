package routing

import (
	"encoding/json"
	"fmt"
	"strings"

	"go-proxy/internal/store"
)

type ChainOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Version    string `json:"version,omitempty"`
}

func AddChain(s *store.Store, tag, server string, port int, username, password string) error {
	if tag == "" || server == "" || port < 1 || port > 65535 {
		return fmt.Errorf("invalid chain parameters")
	}
	for _, raw := range s.SingBox.Outbounds {
		header, err := store.ParseOutboundHeader(raw)
		if err != nil {
			return err
		}
		if header.Tag == tag {
			return fmt.Errorf("outbound %q already exists", tag)
		}
	}
	outbound := ChainOutbound{Type: "socks", Tag: tag, Server: server, ServerPort: port, Username: username, Password: password, Version: "5"}
	raw, err := json.Marshal(outbound)
	if err != nil {
		return err
	}
	s.SingBox.Outbounds = append(s.SingBox.Outbounds, raw)
	s.MarkDirty(store.FileSingBox)
	return nil
}

func RemoveChain(s *store.Store, tag string) error {
	for i, raw := range s.SingBox.Outbounds {
		header, err := store.ParseOutboundHeader(raw)
		if err != nil {
			return err
		}
		if header.Tag == tag && header.Type == "socks" {
			s.SingBox.Outbounds = append(s.SingBox.Outbounds[:i], s.SingBox.Outbounds[i+1:]...)
			s.MarkDirty(store.FileSingBox)
			return nil
		}
	}
	return fmt.Errorf("chain %q not found", tag)
}

func ListChains(s *store.Store) []ChainOutbound {
	chains := []ChainOutbound{}
	for _, raw := range s.SingBox.Outbounds {
		header, err := store.ParseOutboundHeader(raw)
		if err != nil || header.Type != "socks" {
			continue
		}
		var chain ChainOutbound
		if json.Unmarshal(raw, &chain) == nil {
			chains = append(chains, chain)
		}
	}
	return chains
}

const ChainDNSTagPrefix = "gproxy-chain-"

func ChainDNSTag(tag string) string { return ChainDNSTagPrefix + tag }

func isChainDNS(tag string) bool { return strings.HasPrefix(tag, ChainDNSTagPrefix) }
