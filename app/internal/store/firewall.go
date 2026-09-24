package store

import (
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
