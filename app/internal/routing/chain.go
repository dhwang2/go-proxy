package routing

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

// ChainNode represents a socks/http proxy chain outbound.
type ChainNode struct {
	Tag    string
	Type   string // "socks" typically
	Server string
	Port   int
}

// ListChainNodes returns outbounds that are socks/http proxy chain nodes.
func ListChainNodes(st *store.Store) []ChainNode {
	if st == nil || st.Config == nil {
		return nil
	}
	nodes := make([]ChainNode, 0)
	for _, ob := range st.Config.Outbounds {
		typ := strings.TrimSpace(ob.Type)
		if typ != "socks" && typ != "http" {
			continue
		}
		node := ChainNode{
			Tag:  strings.TrimSpace(ob.Tag),
			Type: typ,
		}
		if raw, ok := ob.Raw["server"]; ok {
			_ = json.Unmarshal(raw, &node.Server)
		}
		if raw, ok := ob.Raw["server_port"]; ok {
			_ = json.Unmarshal(raw, &node.Port)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// AddChainNode adds a socks proxy chain outbound.
func AddChainNode(st *store.Store, tag, server string, port int) (MutationResult, error) {
	if st == nil || st.Config == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	tag = strings.TrimSpace(tag)
	server = strings.TrimSpace(server)
	if tag == "" || server == "" || port <= 0 || port > 65535 {
		return MutationResult{}, fmt.Errorf("usage: chain add <tag> <server> <port>")
	}
	// Check for duplicate tag.
	for _, ob := range st.Config.Outbounds {
		if strings.TrimSpace(ob.Tag) == tag {
			return MutationResult{}, fmt.Errorf("outbound already exists: %s", tag)
		}
	}

	raw := map[string]json.RawMessage{}
	writeRaw(raw, "type", "socks")
	writeRaw(raw, "tag", tag)
	writeRaw(raw, "server", server)
	writeRaw(raw, "server_port", port)

	ob := store.Outbound{
		Type: "socks",
		Tag:  tag,
		Raw:  raw,
	}
	st.Config.Outbounds = append(st.Config.Outbounds, ob)
	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		RouteChanged:  1,
	}, nil
}

// RemoveChainNode removes a chain proxy outbound by tag and cleans up
// any route rules that reference the removed outbound.
func RemoveChainNode(st *store.Store, tag string) (MutationResult, error) {
	if st == nil || st.Config == nil {
		return MutationResult{}, fmt.Errorf("store is nil")
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return MutationResult{}, fmt.Errorf("usage: chain remove <tag>")
	}
	next := make([]store.Outbound, 0, len(st.Config.Outbounds))
	removed := false
	for _, ob := range st.Config.Outbounds {
		if strings.TrimSpace(ob.Tag) == tag {
			typ := strings.TrimSpace(ob.Type)
			if typ == "socks" || typ == "http" {
				removed = true
				continue
			}
		}
		next = append(next, ob)
	}
	if !removed {
		return MutationResult{}, fmt.Errorf("chain node not found: %s", tag)
	}
	st.Config.Outbounds = next

	// Remove route rules referencing the deleted outbound.
	routeRemoved := 0
	nextRules := make([]store.RouteRule, 0, len(st.Config.Route.Rules))
	for _, rule := range st.Config.Route.Rules {
		if strings.TrimSpace(rule.Outbound) == tag {
			routeRemoved++
			continue
		}
		nextRules = append(nextRules, rule)
	}
	st.Config.Route.Rules = nextRules

	// Rebuild DNS if route rules were removed.
	dnsChanged := 0
	if routeRemoved > 0 {
		dnsChanged = rebuildManagedDNS(st)
	}

	st.MarkConfigDirty()
	return MutationResult{
		ConfigChanged: true,
		RouteChanged:  routeRemoved + 1,
		DNSChanged:    dnsChanged,
	}, nil
}

func writeRaw(m map[string]json.RawMessage, key string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	m[key] = b
}
