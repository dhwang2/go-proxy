package routing

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ChainResolver is the DNS server a chain's lookups go to, expressed the way
// sing-box accepts it.
//
// Server is always an address, never a name: a chain's DNS server carries a
// detour, and sing-box refuses to resolve the resolver's own hostname through
// one ("missing domain resolver for domain server address"). A name given by
// the operator is resolved once and kept as TLSName, so the certificate is
// still checked against the name while the connection is made to the address.
type ChainResolver struct {
	Type       string
	Server     string
	ServerPort int
	Path       string
	TLSName    string
}

// DefaultChainResolver is what a chain gets when none is chosen: DNS over HTTPS
// to Google, reached through the chain itself so the lookup comes from the same
// address as the traffic.
func DefaultChainResolver() ChainResolver {
	return ChainResolver{Type: "https", Server: "8.8.8.8", ServerPort: 443, Path: "/dns-query", TLSName: "dns.google"}
}

// defaultChainResolverV6 is the same resolver at its IPv6 address.
const defaultChainResolverV6 = "2001:4860:4860::8888"

// DefaultChainResolverFor is the default resolver at an address the chain can
// reach: a chain reached only over IPv6 cannot carry a query to 8.8.8.8, so it
// gets Google's IPv6 address, as shell-proxy chose for an IPv6 endpoint.
func DefaultChainResolverFor(strategy string) ChainResolver {
	resolver := DefaultChainResolver()
	if strategy == "ipv6_only" {
		resolver.Server = defaultChainResolverV6
	}
	return resolver
}

// IsDefaultChainResolver reports whether resolver is the default at either
// address, as opposed to one the operator chose with --dns.
func IsDefaultChainResolver(resolver ChainResolver) bool {
	def := DefaultChainResolver()
	return resolver.Type == def.Type && resolver.ServerPort == def.ServerPort && resolver.Path == def.Path &&
		resolver.TLSName == def.TLSName && (resolver.Server == def.Server || resolver.Server == defaultChainResolverV6)
}

var resolverPorts = map[string]int{"udp": 53, "tcp": 53, "tls": 853, "https": 443, "h3": 443}

// ParseChainResolver reads an operator's spelling of a resolver. A bare address
// is plain DNS; a scheme selects the transport. lookup resolves a hostname to
// an address and may be nil, in which case a hostname is refused rather than
// silently dropped.
func ParseChainResolver(value string, lookup func(string) (net.IP, error)) (ChainResolver, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultChainResolver(), nil
	}
	scheme, rest := "udp", value
	if index := strings.Index(value, "://"); index > 0 {
		scheme, rest = strings.ToLower(value[:index]), value[index+3:]
	}
	port, known := resolverPorts[scheme]
	if !known {
		return ChainResolver{}, fmt.Errorf("unsupported resolver transport %q", scheme)
	}
	path := ""
	if index := strings.Index(rest, "/"); index >= 0 {
		path, rest = rest[index:], rest[:index]
	}
	if scheme == "https" || scheme == "h3" {
		if path == "" {
			path = "/dns-query"
		}
		if _, err := url.Parse(path); err != nil {
			return ChainResolver{}, fmt.Errorf("invalid resolver path")
		}
	} else if path != "" {
		return ChainResolver{}, fmt.Errorf("a path is only meaningful for https or h3")
	}
	host := rest
	if h, p, err := net.SplitHostPort(rest); err == nil {
		parsed, convErr := strconv.Atoi(p)
		if convErr != nil || parsed < 1 || parsed > 65535 {
			return ChainResolver{}, fmt.Errorf("resolver port must be between 1 and 65535")
		}
		host, port = h, parsed
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return ChainResolver{}, fmt.Errorf("resolver needs an address")
	}
	resolver := ChainResolver{Type: scheme, ServerPort: port, Path: path}
	if ip := net.ParseIP(host); ip != nil {
		resolver.Server = host
		return resolver, nil
	}
	if lookup == nil {
		return ChainResolver{}, fmt.Errorf("resolver host must be an ip address")
	}
	address, err := lookup(host)
	if err != nil || address == nil {
		return ChainResolver{}, fmt.Errorf("cannot resolve resolver host")
	}
	// The name is kept for the certificate; the address is what gets dialled.
	resolver.Server, resolver.TLSName = address.String(), host
	return resolver, nil
}
