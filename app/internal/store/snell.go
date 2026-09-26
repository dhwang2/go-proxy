package store

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const SnellTag = "snell-v6"

// The values snell-server v6 accepts for mode and dns-ip-preference, each
// list led by the server's default.
var (
	SnellModes             = []string{"default", "unshaped", "unsafe-raw"}
	SnellDNSIPPreferences  = []string{"default", "prefer-ipv4", "prefer-ipv6", "ipv4-only", "ipv6-only"}
	snellDefaultMode       = SnellModes[0]
	snellDefaultPreference = SnellDNSIPPreferences[0]
)

// SnellConfig represents a snell-v6.conf file, with the keys snell-server v6
// documents in its --help and writes from --wizard.
type SnellConfig struct {
	Listen          string // e.g., "0.0.0.0:8448,[::]:8448"
	PSK             string
	Mode            string // default, unshaped or unsafe-raw
	DNSIPPreference string // default, prefer-ipv4, prefer-ipv6, ipv4-only or ipv6-only
	DNS             string // optional resolver addresses, comma-separated
	EgressInterface string // optional interface outgoing sockets bind to
}

// ParseSnellConfig parses an INI-style snell configuration string. A file
// from before v6 documented dns-ip-preference carries the deprecated ipv6
// key instead, read the way snell-server reads it: false is ipv4-only, true
// the default, and an explicit dns-ip-preference wins over either.
func ParseSnellConfig(data string) (*SnellConfig, error) {
	conf := &SnellConfig{}
	legacyIPv6 := ""
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
		case "mode":
			conf.Mode = val
		case "dns-ip-preference", "ipv-preference":
			conf.DNSIPPreference = val
		case "dns":
			conf.DNS = val
		case "egress-interface":
			conf.EgressInterface = val
		case "ipv6":
			legacyIPv6 = strings.ToLower(val)
		}
	}
	if conf.DNSIPPreference == "" && legacyIPv6 == "false" {
		conf.DNSIPPreference = "ipv4-only"
	}
	conf.Mode, conf.DNSIPPreference = conf.Settings()
	if conf.Listen == "" || conf.PSK == "" {
		return nil, fmt.Errorf("incomplete snell config: listen=%q psk=%q", conf.Listen, conf.PSK)
	}
	return conf, nil
}

// Port extracts the port number from the Listen address.
func (c *SnellConfig) Port() int {
	first, _, _ := strings.Cut(c.Listen, ",")
	_, portStr, err := net.SplitHostPort(strings.TrimSpace(first))
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(portStr)
	return port
}

// Settings are the mode and dns-ip-preference snell-server runs with: an
// unset value is the server's default.
func (c *SnellConfig) Settings() (mode, preference string) {
	mode, preference = c.Mode, c.DNSIPPreference
	if mode == "" {
		mode = snellDefaultMode
	}
	if preference == "" {
		preference = snellDefaultPreference
	}
	return mode, preference
}

// MarshalSnellConfig writes the snell config in the layout snell-server
// --wizard writes: listen, psk, mode and dns-ip-preference, always explicit,
// then dns and egress-interface when they are set.
func (c *SnellConfig) MarshalSnellConfig() []byte {
	mode, preference := c.Settings()
	var b strings.Builder
	fmt.Fprintf(&b, "[snell-server]\nlisten = %s\npsk = %s\nmode = %s\ndns-ip-preference = %s\n", c.Listen, c.PSK, mode, preference)
	if c.DNS != "" {
		fmt.Fprintf(&b, "dns = %s\n", c.DNS)
	}
	if c.EgressInterface != "" {
		fmt.Fprintf(&b, "egress-interface = %s\n", c.EgressInterface)
	}
	return []byte(b.String())
}
