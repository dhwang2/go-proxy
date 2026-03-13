package subscription

import (
	"fmt"
	"net"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

type Target struct {
	Family string
	Host   string
}

type TargetSet struct {
	Preferred Target
	Targets   []Target
}

func DetectTarget(st *store.Store, overrideHost string) (TargetSet, error) {
	overrideHost = normalizeHost(overrideHost)
	if overrideHost != "" {
		t, err := targetFromHost(overrideHost)
		if err != nil {
			return TargetSet{}, err
		}
		return TargetSet{Preferred: t, Targets: []Target{t}}, nil
	}

	domain := ""
	if st != nil && st.Config != nil {
		for _, in := range st.Config.Inbounds {
			tls := parseInboundTLS(in)
			if tls.SNI == "" {
				continue
			}
			host := normalizeHost(tls.SNI)
			if host == "" {
				continue
			}
			if ip := net.ParseIP(host); ip != nil {
				continue
			}
			domain = host
			break
		}
	}

	v4, v6 := detectPublicIPs()
	targets := make([]Target, 0, 3)
	if domain != "" {
		targets = append(targets, Target{Family: "domain", Host: domain})
	}
	if v4 != "" {
		targets = append(targets, Target{Family: "ipv4", Host: v4})
	}
	if v6 != "" {
		targets = append(targets, Target{Family: "ipv6", Host: v6})
	}
	if len(targets) == 0 {
		return TargetSet{}, fmt.Errorf("no subscription target detected; please pass --host")
	}
	return TargetSet{Preferred: targets[0], Targets: targets}, nil
}

func targetFromHost(host string) (Target, error) {
	host = normalizeHost(host)
	if host == "" {
		return Target{}, fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() != nil {
			return Target{Family: "ipv4", Host: ip.String()}, nil
		}
		return Target{Family: "ipv6", Host: ip.String()}, nil
	}
	return Target{Family: "domain", Host: host}, nil
}

func normalizeHost(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	if strings.Contains(v, "://") {
		if p := strings.SplitN(v, "://", 2); len(p) == 2 {
			v = p[1]
		}
	}
	if strings.Contains(v, "/") {
		v = strings.SplitN(v, "/", 2)[0]
	}
	if strings.Contains(v, ":") {
		if host, port, err := net.SplitHostPort(v); err == nil {
			if port != "" {
				v = host
			}
		}
	}
	return strings.TrimSpace(v)
}

func detectPublicIPs() (string, string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	ipv4 := ""
	ipv6 := ""
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if ip == nil || !ip.IsGlobalUnicast() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				if ipv4 == "" && !isPrivateIPv4(ip4) {
					ipv4 = ip4.String()
				}
				continue
			}
			if ipv6 == "" && !isPrivateIPv6(ip) {
				ipv6 = ip.String()
			}
		}
	}
	return ipv4, ipv6
}

func ipFromAddr(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}

func isPrivateIPv4(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip[0] == 10 {
		return true
	}
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	if ip[0] == 127 || ip[0] == 0 || (ip[0] == 169 && ip[1] == 254) {
		return true
	}
	return false
}

func isPrivateIPv6(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return true
	}
	// fc00::/7 unique local addresses.
	if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
		return true
	}
	return false
}
