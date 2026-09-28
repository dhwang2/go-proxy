package subscription

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"

	"go-proxy/internal/config"
)

// Target is one address a link names, and the address families a client may
// use to reach it: "dual", "v4" or "v6", or empty when nobody asked.
type Target struct {
	Family string `json:"family,omitempty"`
	Host   string `json:"host"`
}

// The address modes an export takes: the configured domain, or the server's
// own addresses.
const (
	TargetDomain = "domain"
	TargetIP     = "ip"
)

// Targets are the addresses every link of an export names: the domain, one
// link per node carrying the families it serves, or the server's addresses,
// one link per family.
type Targets struct {
	// Host is what SNI falls back to: the domain, or the first address.
	Host  string   `json:"-"`
	Links []Target `json:"links"`
	// Notes are what the reader should know about the addresses chosen: a
	// family the domain's DNS and the server disagree on, or a domain
	// export on a host with no domain.
	Notes []string `json:"notes,omitempty"`
}

// ResolveTargets reads the addresses for mode. A domain export on a host
// with no configured domain names the server's addresses instead, and says so.
func ResolveTargets(ctx context.Context, mode string) (Targets, error) {
	if mode == TargetDomain {
		if host := readConfiguredDomain(); host != "" {
			records, err := domainFamilies(ctx, host)
			if err != nil {
				return Targets{}, err
			}
			family, notes, err := domainFamily(records, targetFamilies(DetectAddresses(ctx)))
			if err != nil {
				return Targets{}, err
			}
			return Targets{Host: host, Links: []Target{{Host: host, Family: family}}, Notes: notes}, nil
		}
	}
	addresses := DetectAddresses(ctx)
	if len(addresses) == 0 {
		return Targets{}, fmt.Errorf("public address unavailable")
	}
	targets := Targets{Host: addresses[0].Host, Links: addresses}
	if mode == TargetDomain {
		targets.Notes = []string{"no domain is configured, so links name the server's addresses"}
	}
	return targets, nil
}

// DetectAddresses is the server's public address in each family it has,
// IPv4 first. A variable so tests can stand in for the host's network.
var DetectAddresses = detectAddresses

func detectAddresses(ctx context.Context) []Target {
	var v4, v6 string
	done := make(chan struct{})
	go func() { v4 = DetectIPv4(ctx); close(done) }()
	v6 = DetectIPv6(ctx)
	<-done
	targets := make([]Target, 0, 2)
	for _, addr := range []string{v4, v6} {
		if addr != "" {
			targets = append(targets, ipTarget(net.ParseIP(addr)))
		}
	}
	return targets
}

func targetFamilies(targets []Target) []string {
	families := make([]string, 0, len(targets))
	for _, target := range targets {
		families = append(families, target.Family)
	}
	return families
}

// domainFamilies are the families a domain publishes a public address in.
func domainFamilies(ctx context.Context, host string) ([]string, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w; specify --target with an ip address", err)
	}
	seen := map[string]bool{}
	var families []string
	for _, addr := range addrs {
		family := ipTarget(addr.IP).Family
		if !seen[family] && isShareableIP(addr.IP, true) {
			seen[family] = true
			families = append(families, family)
		}
	}
	if len(families) == 0 {
		return nil, fmt.Errorf("target has no public address; specify --target with an ip address")
	}
	return families, nil
}

// domainFamily is the families a link to the domain can use: those the
// domain publishes and the server has. Server addresses that could not be
// detected leave the records to decide. A family on only one side is left
// out, and said.
func domainFamily(records, server []string) (string, []string, error) {
	if len(server) == 0 {
		server = records
	}
	record := map[string]string{"v4": "A", "v6": "AAAA"}
	name := map[string]string{"v4": "ipv4", "v6": "ipv6"}
	var usable, notes []string
	for _, family := range []string{"v4", "v6"} {
		published, present := slices.Contains(records, family), slices.Contains(server, family)
		switch {
		case published && present:
			usable = append(usable, family)
		case present:
			notes = append(notes, fmt.Sprintf("the domain has no %s record, so links leave %s out", record[family], name[family]))
		case published:
			notes = append(notes, fmt.Sprintf("the domain has an %s record but this server has no %s address", record[family], name[family]))
		}
	}
	switch len(usable) {
	case 0:
		return "", notes, fmt.Errorf("the domain publishes no address in a family this server has; fix its dns or specify --target")
	case 1:
		return usable[0], notes, nil
	}
	return "dual", notes, nil
}

func ipTarget(ip net.IP) Target {
	family := "v6"
	if ip.To4() != nil {
		family = "v4"
	}
	return Target{Family: family, Host: ip.String()}
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

func SanitizeServerName(sni string) string {
	sni = strings.TrimSpace(sni)
	sni = strings.TrimPrefix(sni, "https://")
	sni = strings.TrimPrefix(sni, "http://")
	return strings.TrimSuffix(sni, "/")
}
