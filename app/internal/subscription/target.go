package subscription

import (
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"go-proxy/internal/config"
)

// DetectTarget tries to determine the server's public hostname or IP.
// It checks environment, then prefers a configured domain or shareable IP.
func DetectTarget() string {
	if host := strings.TrimSpace(os.Getenv("PROXY_HOST")); host != "" {
		return host
	}

	if domain := readConfiguredDomain(); domain != "" {
		return domain
	}

	if ip := detectPublicIP(); ip != "" {
		return ip
	}
	if ip := detectInterfaceIP(true); ip != "" {
		return ip
	}
	if ip := detectInterfaceIP(false); ip != "" {
		return ip
	}
	if name, err := os.Hostname(); err == nil {
		name = strings.TrimSpace(name)
		if isShareableDomain(name) {
			return name
		}
	}
	return "127.0.0.1"
}

func readConfiguredDomain() string {
	data, err := os.ReadFile(config.DomainFile)
	if err != nil {
		return ""
	}
	domain := SanitizeServerName(string(data))
	if isShareableDomain(domain) {
		return domain
	}
	return ""
}

func detectPublicIP() string {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	for _, endpoint := range []string{
		"https://api.ipify.org",
		"https://ipv4.icanhazip.com",
	} {
		resp, err := client.Get(endpoint)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if isShareableIP(ip, true) {
			return ip.String()
		}
	}
	return ""
}

func detectInterfaceIP(publicOnly bool) string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var fallback string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP == nil || ipnet.IP.IsLoopback() {
			continue
		}
		ip := ipnet.IP
		if ip.To4() == nil {
			continue
		}
		if !isShareableIP(ip, publicOnly) {
			continue
		}
		if publicOnly {
			return ip.String()
		}
		if fallback == "" {
			fallback = ip.String()
		}
	}
	if fallback != "" {
		return fallback
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP == nil || ipnet.IP.IsLoopback() {
			continue
		}
		ip := ipnet.IP
		if ip.To4() != nil {
			continue
		}
		if isShareableIP(ip, publicOnly) {
			return ip.String()
		}
	}
	return ""
}

func isShareableIP(ip net.IP, publicOnly bool) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if publicOnly {
		return !ip.IsPrivate()
	}
	return !ip.IsUnspecified() && !ip.IsMulticast()
}

func isShareableDomain(host string) bool {
	host = SanitizeServerName(host)
	return host != "" && net.ParseIP(host) == nil && strings.Contains(host, ".")
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
