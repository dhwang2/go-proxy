package routing

import (
	"encoding/json"
	"net"
	"reflect"
	"sort"
	"strconv"

	"go-proxy/internal/store"
	"go-proxy/pkg/jsonorder"
)

func Sync(s *store.Store) {
	rules := CompiledUserRouteRules(s)
	strategy := ""
	if s.SingBox.DNS != nil {
		strategy = s.SingBox.DNS.Strategy
	}
	syncDNS(s, nil, strategy, rules)
	syncRouteRules(s, rules)
}

func syncDNS(s *store.Store, outboundToDNS map[string]string, strategy string, rules []store.RouteRule) {
	if s.SingBox.DNS == nil {
		return
	}

	// Build outbound-to-DNS mapping from chain proxy outbounds.
	if outboundToDNS == nil {
		outboundToDNS = buildOutboundToDNS(s)
	}

	// Sync DNS servers: ensure each chain outbound has a detour DNS server.
	syncChainDNSServers(s)
	if final := DNSFinal(s); s.SingBox.DNS.Final != final {
		s.SingBox.DNS.Final = final
		s.MarkDirty(store.FileSingBox)
	}

	// Keep non-auth_user DNS rules.
	var kept []store.DNSRule
	for _, r := range s.SingBox.DNS.Rules {
		if len(r.AuthUser) == 0 {
			kept = append(kept, r)
		}
	}

	newRules := mergeDNSRulesByServer(dnsRulesFromRouteRules(rules, outboundToDNS, ChainDNSStrategies(s), strategy))
	kept = append(kept, newRules...)

	s.SingBox.DNS.Rules = kept
	s.MarkDirty(store.FileSingBox)
}

func syncRouteRules(s *store.Store, rules []store.RouteRule) {
	if s.SingBox.Route == nil {
		s.SingBox.Route = &store.RouteConfig{}
	}

	// Separate base rules (non-auth_user) from user rules.
	var base []store.RouteRule
	for _, r := range s.SingBox.Route.Rules {
		if len(r.AuthUser) == 0 {
			base = append(base, r)
		}
	}

	s.SingBox.Route.Rules = append(base, rules...)
	s.SingBox.EnsureDefaultDomainResolver()
	s.MarkDirty(store.FileSingBox)
}

func buildOutboundToDNS(s *store.Store) map[string]string {
	directDNS := "public4"
	if s.SingBox.Route != nil && s.SingBox.Route.DefaultDomainResolver != "" {
		directDNS = s.SingBox.Route.DefaultDomainResolver
	} else if s.SingBox.DNS != nil && s.SingBox.DNS.Final != "" {
		directDNS = s.SingBox.DNS.Final
	}
	m := map[string]string{store.DirectTag: directDNS}
	for _, raw := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(raw)
		if h.Type == "socks" {
			m[h.Tag] = ChainDNSTag(h.Tag)
		}
	}
	return m
}

// chainDNSServer is a chain's DNS server in shell-proxy's shape: where the
// resolver is and the chain it is reached through. The families its lookups ask
// for are not here; they are set on the DNS rules, from ChainStrategies.
type chainDNSServer struct {
	Tag        string         `json:"tag"`
	Type       string         `json:"type"`
	Server     string         `json:"server"`
	ServerPort int            `json:"server_port"`
	Path       string         `json:"path,omitempty"`
	TLS        map[string]any `json:"tls,omitempty"`
	Detour     string         `json:"detour"`
}

// chainEndpoints maps each chain's tag to the address it dials.
func chainEndpoints(s *store.Store) map[string]string {
	endpoints := map[string]string{}
	for _, raw := range s.SingBox.Outbounds {
		h, _ := store.ParseOutboundHeader(raw)
		if h.Type != "socks" {
			continue
		}
		var chain ChainOutbound
		if json.Unmarshal(raw, &chain) == nil {
			endpoints[h.Tag] = chain.Server
		}
	}
	return endpoints
}

// syncChainDNSServers gives every chain one DNS server and drops the servers of
// chains that are gone. It also brings a server written by an earlier gproxy
// into the current shape, keeping every other key it carries in its order:
//   - a legacy "gproxy-chain-<tag>" name becomes "<tag>-dns";
//   - a domain_strategy field moves into the chain strategy record;
//   - the default resolver is moved to the address family the chain reaches,
//     so an IPv6-only chain stops sending its lookups to 8.8.8.8.
func syncChainDNSServers(s *store.Store) {
	if s.SingBox.DNS == nil {
		return
	}
	endpoints := chainEndpoints(s)
	if s.UserMeta != nil {
		for tag := range s.UserMeta.ChainStrategy {
			if _, ok := endpoints[tag]; !ok {
				delete(s.UserMeta.ChainStrategy, tag)
				s.MarkDirty(store.FileUserMeta)
			}
		}
	}

	var kept []json.RawMessage
	existing := make(map[string]bool)
	for _, raw := range s.SingBox.DNS.Servers {
		var srv chainDNSServer
		var legacy struct {
			DomainStrategy string `json:"domain_strategy"`
		}
		if json.Unmarshal(raw, &srv) != nil || !isChainDNS(srv.Tag, srv.Detour) {
			kept = append(kept, raw)
			continue
		}
		if _, ok := endpoints[srv.Detour]; !ok {
			s.MarkDirty(store.FileSingBox)
			continue
		}
		_ = json.Unmarshal(raw, &legacy)
		if legacy.DomainStrategy != "" && recordedStrategy(s, srv.Detour) == "" {
			recordChainStrategy(s, srv.Detour, legacy.DomainStrategy)
		}
		strategy := chainStrategyOf(s, srv.Detour, endpoints[srv.Detour])
		resolver := ChainResolver{Type: srv.Type, Server: srv.Server, ServerPort: srv.ServerPort, Path: srv.Path}
		if name, ok := srv.TLS["server_name"].(string); ok {
			resolver.TLSName = name
		}
		want := resolver.Server
		if IsDefaultChainResolver(resolver) {
			want = DefaultChainResolverFor(strategy).Server
		}
		value, err := jsonorder.Parse(raw)
		if err != nil {
			kept = append(kept, raw)
			continue
		}
		changed := false
		if srv.Tag != ChainDNSTag(srv.Detour) {
			value.Set("tag", jsonorder.String(ChainDNSTag(srv.Detour)))
			changed = true
		}
		if value.Get("domain_strategy") != nil {
			value.Delete("domain_strategy")
			changed = true
		}
		if want != srv.Server {
			value.Set("server", jsonorder.String(want))
			changed = true
		}
		if changed {
			if encoded, err := value.MarshalJSON(); err == nil {
				raw = encoded
				s.MarkDirty(store.FileSingBox)
			}
		}
		if existing[ChainDNSTag(srv.Detour)] {
			continue
		}
		existing[ChainDNSTag(srv.Detour)] = true
		kept = append(kept, raw)
	}

	tags := make([]string, 0, len(endpoints))
	for tag := range endpoints {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		if existing[ChainDNSTag(tag)] {
			continue
		}
		strategy := chainStrategyOf(s, tag, endpoints[tag])
		raw, err := json.Marshal(chainDNSServerFor(tag, DefaultChainResolverFor(strategy)))
		if err != nil {
			continue
		}
		kept = append(kept, raw)
		s.MarkDirty(store.FileSingBox)
	}

	s.SingBox.DNS.Servers = kept
}

// ChainStrategy picks the address families a chain's lookups ask for, from the
// address it dials. An endpoint reached over IPv4 cannot carry a AAAA answer
// and one reached over IPv6 cannot carry an A answer, so each asks for its own
// family alone: a preference would still allow the other, and the answer would
// be an address the chain cannot reach.
//
// A bare hostname is decided where it can be resolved, not here.
func ChainStrategy(server string) string {
	ip := net.ParseIP(server)
	switch {
	case ip == nil:
		return ""
	case ip.To4() != nil:
		return "ipv4_only"
	default:
		return "ipv6_only"
	}
}

func recordedStrategy(s *store.Store, tag string) string {
	if s.UserMeta == nil {
		return ""
	}
	return s.UserMeta.ChainStrategy[tag]
}

// recordChainStrategy keeps what a lookup decided for a chain; an empty
// strategy (a hostname that did not resolve) removes the record.
func recordChainStrategy(s *store.Store, tag, strategy string) {
	if s.UserMeta == nil {
		return
	}
	if strategy == "" {
		if _, ok := s.UserMeta.ChainStrategy[tag]; ok {
			delete(s.UserMeta.ChainStrategy, tag)
			s.MarkDirty(store.FileUserMeta)
		}
		return
	}
	if s.UserMeta.ChainStrategy == nil {
		s.UserMeta.ChainStrategy = map[string]string{}
	}
	if s.UserMeta.ChainStrategy[tag] != strategy {
		s.UserMeta.ChainStrategy[tag] = strategy
		s.MarkDirty(store.FileUserMeta)
	}
}

// chainStrategyOf is a chain's strategy: its own address decides it when the
// endpoint is an address, and the recorded lookup when it is a hostname.
func chainStrategyOf(s *store.Store, tag, endpoint string) string {
	if strategy := ChainStrategy(endpoint); strategy != "" {
		return strategy
	}
	return recordedStrategy(s, tag)
}

// ChainStrategies maps each chain's tag to its strategy, "" where none is
// known, so its DNS rules ask for the families its endpoint can reach.
func ChainStrategies(s *store.Store) map[string]string {
	out := map[string]string{}
	for tag, endpoint := range chainEndpoints(s) {
		out[tag] = chainStrategyOf(s, tag, endpoint)
	}
	return out
}

// ChainDNSStrategies is ChainStrategies keyed by each chain's DNS server tag,
// which is what a compiled DNS rule names.
func ChainDNSStrategies(s *store.Store) map[string]string {
	out := map[string]string{}
	for tag, strategy := range ChainStrategies(s) {
		if strategy != "" {
			out[ChainDNSTag(tag)] = strategy
		}
	}
	return out
}

// chainDNSServerFor builds the DNS server entry for one chain. TLS is only
// declared for the encrypted transports, and only with a name when one was
// given: a certificate cannot be checked against a bare address.
func chainDNSServerFor(chainTag string, resolver ChainResolver) chainDNSServer {
	srv := chainDNSServer{
		Tag: ChainDNSTag(chainTag), Type: resolver.Type, Server: resolver.Server, ServerPort: resolver.ServerPort,
		Path: resolver.Path, Detour: chainTag,
	}
	if resolver.TLSName != "" && (resolver.Type == "https" || resolver.Type == "h3" || resolver.Type == "tls") {
		srv.TLS = map[string]any{"enabled": true, "server_name": resolver.TLSName}
	}
	return srv
}

// SetChainDNS writes the chain's DNS server and records its strategy. Both
// need the network to decide -- a hostname resolver and a hostname endpoint
// each take a lookup -- and the compile path that rebuilds the rules has none,
// so the answers are recorded here where they were found.
func SetChainDNS(s *store.Store, tag string, resolver ChainResolver, strategy string) {
	if s.SingBox.DNS == nil {
		return
	}
	recordChainStrategy(s, tag, strategy)
	srv := chainDNSServerFor(tag, resolver)
	raw, err := json.Marshal(srv)
	if err != nil {
		return
	}
	for index, existing := range s.SingBox.DNS.Servers {
		var current chainDNSServer
		if json.Unmarshal(existing, &current) != nil || current.Detour != tag || !isChainDNS(current.Tag, current.Detour) {
			continue
		}
		// Rewriting the same server is not a change. Compared as values:
		// the stored copy came back from an indented file, so equal
		// settings never have equal encodings.
		if reflect.DeepEqual(current, srv) {
			var extra struct {
				DomainStrategy string `json:"domain_strategy"`
			}
			if json.Unmarshal(existing, &extra) == nil && extra.DomainStrategy == "" {
				return
			}
		}
		s.SingBox.DNS.Servers[index] = raw
		s.MarkDirty(store.FileSingBox)
		return
	}
	s.SingBox.DNS.Servers = append(s.SingBox.DNS.Servers, raw)
	s.MarkDirty(store.FileSingBox)
}

// ChainResolverOf reconstructs the resolver a chain's DNS server was built
// from, so a change to the chain can rebuild that server without asking for the
// resolver again. The stored entry is the only record of it.
func ChainResolverOf(s *store.Store, tag string) (ChainResolver, bool) {
	if s.SingBox.DNS == nil {
		return ChainResolver{}, false
	}
	for _, raw := range s.SingBox.DNS.Servers {
		var current chainDNSServer
		if json.Unmarshal(raw, &current) != nil || current.Detour != tag || !isChainDNS(current.Tag, current.Detour) {
			continue
		}
		resolver := ChainResolver{Type: current.Type, Server: current.Server, ServerPort: current.ServerPort, Path: current.Path}
		if name, ok := current.TLS["server_name"].(string); ok {
			resolver.TLSName = name
		}
		return resolver, true
	}
	return ChainResolver{}, false
}

// ChainResolvers reports, per chain tag, the resolver its DNS server points
// at, for display.
func ChainResolvers(s *store.Store) map[string]string {
	out := map[string]string{}
	if s.SingBox.DNS == nil {
		return out
	}
	for _, raw := range s.SingBox.DNS.Servers {
		var srv chainDNSServer
		if json.Unmarshal(raw, &srv) != nil || !isChainDNS(srv.Tag, srv.Detour) {
			continue
		}
		address := net.JoinHostPort(srv.Server, strconv.Itoa(srv.ServerPort))
		if name, _ := srv.TLS["server_name"].(string); name != "" {
			address = name + " " + address
		}
		out[srv.Detour] = srv.Type + " " + address
	}
	return out
}

// DirectResolverFor names the DNS server direct lookups go to under strategy:
// the IPv6 resolver when IPv6 is required or preferred, the IPv4 one otherwise.
func DirectResolverFor(strategy string) string {
	switch strategy {
	case "ipv6_only", "prefer_ipv6":
		return "public6"
	}
	return "public4"
}

// ApplyDirectStrategy sets the address families direct traffic resolves in,
// everywhere sing-box reads that choice: the DNS default, the DNS final
// server, the route's default resolver and the direct outbound's own resolver.
// Setting only dns.strategy left the other three on IPv4, so a dual-stack or
// IPv6-only host still resolved direct traffic through the IPv4 resolver.
//
// An empty strategy is "asis": sing-box decides, through the IPv4 resolver.
// The user rules are recompiled, because a direct rule asks for this strategy.
func ApplyDirectStrategy(s *store.Store, strategy string) {
	if s.SingBox.DNS == nil {
		s.SingBox.DNS = &store.DNSConfig{}
	}
	if s.SingBox.Route == nil {
		s.SingBox.Route = &store.RouteConfig{}
	}
	resolver := DirectResolverFor(strategy)
	s.SingBox.DNS.Strategy = strategy
	s.SingBox.DNS.Final = DNSFinal(s)
	s.SingBox.Route.DefaultDomainResolver = resolver
	s.SingBox.SetDirectResolver(resolver, strategy)
	s.MarkDirty(store.FileSingBox)
	Sync(s)
}

// DirectStrategyResolved reports whether this configuration has had its
// direct strategy applied. One written before go-proxy chose it per host has
// no resolver on the direct outbound.
func DirectStrategyResolved(s *store.Store) bool {
	_, _, ok := s.SingBox.DirectResolver()
	return ok
}

// unchosenStrategy is the fixed default configurations were written with
// before the strategy followed the host's addresses. The bootstrap default
// still carries it, so a fresh configuration reads as unchosen as well.
const unchosenStrategy = "ipv4_only"

// ResolveDirectStrategy completes a configuration whose direct strategy was
// never applied, and reports whether it changed anything. The old fixed
// default is replaced by detected, the strategy this host's addresses call
// for. Any other strategy was chosen by someone and is kept, with the
// resolvers brought into line with it. An empty detected means the addresses
// could not be read; the old default then stays, to be resolved next time.
func ResolveDirectStrategy(s *store.Store, detected string) bool {
	if DirectStrategyResolved(s) {
		return false
	}
	strategy := ""
	if s.SingBox.DNS != nil {
		strategy = s.SingBox.DNS.Strategy
	}
	if strategy == unchosenStrategy {
		if detected == "" {
			return false
		}
		strategy = detected
	}
	ApplyDirectStrategy(s, strategy)
	return true
}

// DNSFinal names the DNS server for lookups no rule claims. When every
// unclaimed connection leaves through a chain -- route.final set to it,
// shell-proxy's whole-server chain mode -- that is the chain's own server, so a
// name is resolved from where it will be dialled. Otherwise it is the direct
// resolver for the configured strategy. Direct rules keep resolving through
// route.default_domain_resolver either way.
func DNSFinal(s *store.Store) string {
	if s.SingBox.Route != nil {
		if _, ok := chainEndpoints(s)[s.SingBox.Route.Final]; ok {
			return ChainDNSTag(s.SingBox.Route.Final)
		}
	}
	strategy := ""
	if s.SingBox.DNS != nil {
		strategy = s.SingBox.DNS.Strategy
	}
	return DirectResolverFor(strategy)
}

// RouteFinal is where connections no rule claims leave: "direct", or a chain.
func RouteFinal(s *store.Store) string {
	if s.SingBox.Route == nil || s.SingBox.Route.Final == "" {
		return store.DirectTag
	}
	return OutboundLabel(s.SingBox.Route.Final)
}
