package subscription

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"go-proxy/internal/config"
)

type SurgeTarget struct {
	Family string `json:"family"`
	Host   string `json:"host"`
}

func ResolveTargets(ctx context.Context, explicit string, resolveFamilies bool) (string, []SurgeTarget, error) {
	host := explicit
	if host == "" {
		host = strings.TrimSpace(os.Getenv("PROXY_HOST"))
	}
	if host == "" {
		host = readConfiguredDomain()
	}
	if host != "" {
		if ip := net.ParseIP(host); ip != nil {
			return host, []SurgeTarget{ipTarget(ip)}, nil
		}
		if !isShareableDomain(host) {
			return "", nil, fmt.Errorf("invalid target host")
		}
		if !resolveFamilies {
			return host, []SurgeTarget{{Host: host}}, nil
		}
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return "", nil, fmt.Errorf("resolve target: %w; specify --target with an ip address", err)
		}
		targets := make([]SurgeTarget, 0, 2)
		seen := map[string]bool{}
		for _, addr := range addrs {
			target := ipTarget(addr.IP)
			if !seen[target.Family] && isShareableIP(addr.IP, true) {
				targets = append(targets, target)
				seen[target.Family] = true
			}
		}
		if len(targets) == 0 {
			return "", nil, fmt.Errorf("target has no public address; specify --target with an ip address")
		}
		return host, targets, nil
	}
	var v4, v6 string
	done := make(chan struct{})
	go func() { v4 = DetectIPv4(ctx); close(done) }()
	v6 = DetectIPv6(ctx)
	<-done
	targets := make([]SurgeTarget, 0, 2)
	for _, addr := range []string{v4, v6} {
		if addr != "" {
			targets = append(targets, ipTarget(net.ParseIP(addr)))
		}
	}
	if len(targets) == 0 {
		return "", nil, fmt.Errorf("public target unavailable; specify --target")
	}
	return targets[0].Host, targets, nil
}

func ipTarget(ip net.IP) SurgeTarget {
	family := "v6"
	if ip.To4() != nil {
		family = "v4"
	}
	return SurgeTarget{Family: family, Host: ip.String()}
}

func DetectIPv4(ctx context.Context) string {
	if ip := routeSourceIP(ctx, "udp4", "1.1.1.1"); isPublicIPv4(ip) {
		return ip
	}
	for _, endpoint := range []string{"https://api.ipify.org", "https://ipv4.icanhazip.com"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return ""
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if readErr == nil && resp.StatusCode == http.StatusOK {
			ip := strings.TrimSpace(string(body))
			if isPublicIPv4(ip) {
				return ip
			}
		}
	}
	return ""
}

func DetectIPv6(ctx context.Context) string {
	if ip := routeSourceIP(ctx, "udp6", "2606:4700:4700::1111"); isShareableIPv6(ip) {
		return ip
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && isShareableIPv6(ip.IP.String()) {
			return ip.IP.String()
		}
	}
	return ""
}

func routeSourceIP(ctx context.Context, network, dst string) string {
	conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(dst, "53"))
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func readConfiguredDomain() string {
	data, err := os.ReadFile(config.DomainFile)
	if err != nil {
		return ""
	}
	domain := strings.TrimSpace(string(data))
	if isShareableDomain(domain) {
		return domain
	}
	return ""
}

func isPublicIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil && isShareableIP(ip, true)
}

func isShareableIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil && isShareableIP(ip, true)
}

func isShareableIP(ip net.IP, publicOnly bool) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast() && (!publicOnly || !ip.IsPrivate())
}

func isShareableDomain(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func IsIPv6(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() == nil
}

func FormatHost(host string) string {
	if IsIPv6(host) {
		return "[" + host + "]"
	}
	return host
}

func SanitizeServerName(sni string) string {
	sni = strings.TrimSpace(sni)
	sni = strings.TrimPrefix(sni, "https://")
	sni = strings.TrimPrefix(sni, "http://")
	return strings.TrimSuffix(sni, "/")
}
