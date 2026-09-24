package routing

import (
	"encoding/json"
	"fmt"

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

// ReplaceChain rewrites an existing chain outbound in place, keeping its
// position so the rules that select it by tag are untouched.
func ReplaceChain(s *store.Store, desired ChainOutbound) error {
	if desired.Tag == "" || desired.Server == "" || desired.ServerPort < 1 || desired.ServerPort > 65535 {
		return fmt.Errorf("invalid chain parameters")
	}
	raw, err := json.Marshal(desired)
	if err != nil {
		return err
	}
	for i, existing := range s.SingBox.Outbounds {
		header, err := store.ParseOutboundHeader(existing)
		if err != nil {
			return err
		}
		if header.Tag == desired.Tag && header.Type == "socks" {
			// Compared as values, not as bytes: what is stored came back from a
			// file written with indentation, so identical settings never have
			// identical encodings.
			var current ChainOutbound
			if json.Unmarshal(existing, &current) == nil && current == desired {
				return nil
			}
			s.SingBox.Outbounds[i] = raw
			s.MarkDirty(store.FileSingBox)
			return nil
		}
	}
	return fmt.Errorf("chain %q not found", desired.Tag)
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

// ChainDNSTag names a chain's DNS server: the chain's tag with "-dns". The
// server carries the shell-proxy fields only -- tag, type, address, TLS and the
// detour through the chain -- and the families its lookups ask for are set on
// the DNS rules that use it.
func ChainDNSTag(tag string) string { return tag + "-dns" }

// legacyChainDNSPrefix is how chain DNS servers were named before u-2-144.
// A sync renames them.
const legacyChainDNSPrefix = "gproxy-chain-"

// isChainDNS reports whether a DNS server belongs to the chain its detour
// names, under the current name or the legacy one.
func isChainDNS(tag, detour string) bool {
	return detour != "" && (tag == ChainDNSTag(detour) || tag == legacyChainDNSPrefix+detour)
}
