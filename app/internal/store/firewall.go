package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type FirewallPort struct {
	Proto string `json:"proto"`
	Port  int    `json:"port"`
}

// FirewallConfig stores additional custom ports that should remain open.
type FirewallConfig struct {
	Ports []FirewallPort `json:"ports,omitempty"`
}

func (c *FirewallConfig) UnmarshalJSON(data []byte) error {
	type rawFirewallConfig struct {
		Ports []FirewallPort  `json:"ports"`
		TCP   json.RawMessage `json:"tcp"`
		UDP   json.RawMessage `json:"udp"`
	}
	var raw rawFirewallConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if legacyFieldPresent(raw.TCP) || legacyFieldPresent(raw.UDP) {
		return fmt.Errorf("legacy firewall config format is no longer supported")
	}
	c.Ports = raw.Ports
	return nil
}

func (c *FirewallConfig) Normalize() {
	if c == nil {
		return
	}
	if len(c.Ports) == 0 {
		c.Ports = nil
		return
	}
	seen := make(map[string]bool, len(c.Ports))
	ports := make([]FirewallPort, 0, len(c.Ports))
	for _, entry := range c.Ports {
		if entry.Port <= 0 || entry.Port > 65535 {
			continue
		}
		entry.Proto = normalizeFirewallPortProto(entry.Proto)
		key := fmt.Sprintf("%s/%d", entry.Proto, entry.Port)
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, entry)
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Port != ports[j].Port {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Proto < ports[j].Proto
	})
	c.Ports = ports
}

func normalizeFirewallPortProto(proto string) string {
	if strings.EqualFold(strings.TrimSpace(proto), "udp") {
		return "udp"
	}
	return "tcp"
}

func legacyFieldPresent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	return !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
