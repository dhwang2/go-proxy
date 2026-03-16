package subscription

import (
	"net"
	"os"
	"strings"
)

// DetectTarget tries to determine the server's public hostname or IP.
// It checks environment, then falls back to hostname.
func DetectTarget() string {
	// Check environment variable first.
	if host := os.Getenv("PROXY_HOST"); host != "" {
		return host
	}

	// Try hostname.
	if name, err := os.Hostname(); err == nil && name != "" {
		return name
	}

	// Fall back to first non-loopback IP.
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// IsIPv6 returns true if the address is an IPv6 address.
func IsIPv6(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() == nil
}

// FormatHost wraps IPv6 addresses in brackets for URL use.
func FormatHost(host string) string {
	if IsIPv6(host) {
		return "[" + host + "]"
	}
	return host
}

// SanitizeServerName returns a clean server name for TLS.
func SanitizeServerName(sni string) string {
	sni = strings.TrimSpace(sni)
	sni = strings.TrimPrefix(sni, "https://")
	sni = strings.TrimPrefix(sni, "http://")
	sni = strings.TrimSuffix(sni, "/")
	return sni
}
