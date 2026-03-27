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

// SurgeTarget holds a resolved host and its IP family for Surge subscription rendering.
type SurgeTarget struct {
	Family string // "v4" or "v6"
	Host   string // IP address (IPv4 or IPv6)
}

// DetectIPv4 detects the server's public IPv4 address.
func DetectIPv4() string {
	// Try routing table first.
	if ip := routeSourceIP("udp4", "1.1.1.1"); isPublicIPv4(ip) {
		return ip
	}
	// Fall back to HTTP detection.
	client := &http.Client{Timeout: 2 * time.Second}
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
		if ip != nil && ip.To4() != nil && isPublicIPv4(ip.String()) {
			return ip.String()
		}
	}
	return ""
}

// DetectIPv6 detects the server's public IPv6 address.
func DetectIPv6() string {
	// Quick check: skip if no global IPv6 interface exists.
	ifaceIPv6 := detectGlobalIPv6Interface()
	if ifaceIPv6 == "" {
		return ""
	}
	// Try routing table first.
	if ip := routeSourceIP("udp6", "2606:4700:4700::1111"); isShareableIPv6(ip) {
		return ip
	}
	// Use the interface address found above.
	if isShareableIPv6(ifaceIPv6) {
		return ifaceIPv6
	}
	// Fall back to HTTP detection.
	client := &http.Client{Timeout: 2 * time.Second}
	for _, endpoint := range []string{
		"https://api64.ipify.org",
		"https://ipv6.icanhazip.com",
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
		if ip != nil && ip.To4() == nil && isShareableIPv6(ip.String()) {
			return ip.String()
		}
	}
	return ""
}

// DetectSurgeTargets returns the list of IP-based targets for Surge subscription rendering.
// For single-stack, returns one target with no suffix. For dual-stack, returns two with -v4/-v6 suffixes.
// If host is an IP literal, it is returned directly without detection.
func DetectSurgeTargets(host string) []SurgeTarget {
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.To4() != nil {
			return []SurgeTarget{{Family: "v4", Host: host}}
		}
		return []SurgeTarget{{Family: "v6", Host: host}}
	}

	// host is a domain — try to resolve it to IPs.
	var ipv4, ipv6 string
	dnsResolved := false

	addrs, err := net.LookupIP(host)
	if err == nil && len(addrs) > 0 {
		dnsResolved = true
		for _, a := range addrs {
			if a.To4() != nil && ipv4 == "" && isPublicIPv4(a.String()) {
				ipv4 = a.String()
			} else if a.To4() == nil && ipv6 == "" && isShareableIPv6(a.String()) {
				ipv6 = a.String()
			}
		}
	}

	// Fall back to server-local detection only when DNS failed entirely.
	// Don't fabricate a second family from server-local detection.
	if !dnsResolved {
		ipv4 = DetectIPv4()
		ipv6 = DetectIPv6()
	}

	var targets []SurgeTarget
	if ipv4 != "" {
		targets = append(targets, SurgeTarget{Family: "v4", Host: ipv4})
	}
	if ipv6 != "" {
		targets = append(targets, SurgeTarget{Family: "v6", Host: ipv6})
	}
	if len(targets) == 0 {
		// Last resort: return the original host as-is with no suffix.
		return []SurgeTarget{{Family: "", Host: host}}
	}
	return targets
}

// routeSourceIP uses a UDP connect trick to determine the local source IP for reaching dst.
func routeSourceIP(network, dst string) string {
	conn, err := net.Dial(network, net.JoinHostPort(dst, "53"))
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return localAddr.IP.String()
}

// detectGlobalIPv6Interface returns the first global IPv6 address from interfaces.
func detectGlobalIPv6Interface() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP == nil {
			continue
		}
		if ipnet.IP.To4() == nil && isShareableIPv6(ipnet.IP.String()) {
			return ipnet.IP.String()
		}
	}
	return ""
}

// isPublicIPv4 returns true if the string is a public (non-private, non-special) IPv4 address.
func isPublicIPv4(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return isShareableIP(ip, true)
}

// isShareableIPv6 returns true if the string is a global-unicast, non-link-local IPv6.
func isShareableIPv6(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() != nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	// Exclude ULA (fc00::/7) — fc and fd prefixes are private.
	if len(ip) == 16 && (ip[0]&0xfe) == 0xfc {
		return false
	}
	return true
}

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
