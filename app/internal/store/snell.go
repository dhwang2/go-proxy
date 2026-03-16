package store

import (
	"fmt"
	"strings"
)

// SnellConfig represents a snell-v5.conf file.
type SnellConfig struct {
	Listen string // e.g., "0.0.0.0:8448"
	PSK    string
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
		}
	}
	if conf.Listen == "" || conf.PSK == "" {
		return nil, fmt.Errorf("incomplete snell config: listen=%q psk=%q", conf.Listen, conf.PSK)
	}
	return conf, nil
}

// MarshalSnellConfig writes the snell config to INI format.
func (c *SnellConfig) MarshalSnellConfig() []byte {
	return []byte(fmt.Sprintf("[snell server]\nlisten = %s\npsk = %s\n", c.Listen, c.PSK))
}
