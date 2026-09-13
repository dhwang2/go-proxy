package store

import (
	"fmt"
	"strings"
)

const SnellTag = "snell-v6"

// SnellConfig represents a snell-v6.conf file.
type SnellConfig struct {
	Listen string // e.g., "0.0.0.0:8448"
	PSK    string
	IPv6   bool
}

// ParseSnellConfig parses an INI-style snell configuration string.
func ParseSnellConfig(data string) (*SnellConfig, error) {
	conf := &SnellConfig{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch key {
		case "listen":
			conf.Listen = val
		case "psk":
			conf.PSK = val
		case "ipv6":
			conf.IPv6 = strings.EqualFold(val, "true")
		}
	}
	if conf.Listen == "" || conf.PSK == "" {
		return nil, fmt.Errorf("incomplete snell config: listen=%q psk=%q", conf.Listen, conf.PSK)
	}
	return conf, nil
}

// Port extracts the port number from the Listen address.
func (c *SnellConfig) Port() int {
	_, portStr, ok := strings.Cut(c.Listen, ":")
	if !ok {
		return 0
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// MarshalSnellConfig writes the snell config to INI format.
func (c *SnellConfig) MarshalSnellConfig() []byte {
	return []byte(fmt.Sprintf("[snell-server]\nlisten = %s\npsk = %s\nipv6 = %t\n",
		c.Listen, c.PSK, c.IPv6))
}
