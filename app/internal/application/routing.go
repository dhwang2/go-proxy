package application

import (
	"context"
	"encoding/json"
	"net"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go-proxy/internal/derived"
	"go-proxy/internal/network"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

type RouteEntry struct {
	User  string `json:"user"`
	Index int    `json:"index"`
	Label string `json:"label"`
	// Preset is the preset the rule was made from, "" for a custom rule.
	Preset string `json:"preset,omitempty"`
	// Selector is what `route rule modify|remove --rules` takes for this rule:
	// the preset's menu index, or cN for the Nth rule no preset made.
	Selector string `json:"selector"`
	// Outbound is the normalised tag the rule sends traffic to and Address the
	// server behind it. A tag on its own does not say where traffic goes, so a
	// reader would have to run route chain list to finish reading one rule.
	Outbound string `json:"outbound"`
	Address  string `json:"address,omitempty"`
	// Rule is the stored rule, with its outbound named the way the caller named
	// it. Everything else is verbatim; only the decorated internal spelling of
	// direct is translated, because nothing outside sing-box should have to
	// recognise it.
	Rule store.UserRouteRule `json:"rule"`
}

// routeEntries numbers each user's rules from one, which is the numbering
// `route rule remove --rules` expects. One continuous numbering across users
// would print indexes that command rejects.
func routeEntries(s *store.Store, only string) []RouteEntry {
	addresses := map[string]string{}
	for _, chain := range routing.ListChains(s) {
		addresses[chain.Tag] = chainAddress(chain)
	}
	entries := []RouteEntry{}
	counts := map[string]int{}
	customs := map[string]int{}
	for _, rule := range s.UserRoutes {
		for _, user := range rule.AuthUser {
			if only != "" && user != only {
				continue
			}
			counts[user]++
			// rule is this loop's copy, so naming its outbound here leaves the
			// stored rule alone.
			shown := rule
			shown.Outbound = routing.OutboundLabel(rule.Outbound)
			preset := routing.UserRoutePreset(rule)
			selector := routing.PresetSymbol(preset)
			if selector == "" {
				customs[user]++
				selector = "c" + strconv.Itoa(customs[user])
			}
			entries = append(entries, RouteEntry{
				User:     user,
				Index:    counts[user],
				Label:    routing.UserRouteLabel(rule),
				Preset:   preset,
				Selector: selector,
				Outbound: shown.Outbound,
				Address:  addresses[rule.Outbound],
				Rule:     shown,
			})
		}
	}
	// Each user's rules read in menu order, as the menu and shell-proxy's
	// listing do; rules no preset made follow, in the order they were added.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].User != entries[j].User {
			return false
		}
		return routing.PresetRank(entries[i].Preset) < routing.PresetRank(entries[j].Preset)
	})
	return entries
}

func (a *App) RoutingList(ctx context.Context, name string) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	if name != "" {
		if err := routingUserExists(snapshot.Store, name); err != nil {
			return Result{}, err
		}
	}
	return Result{Data: map[string]any{"rules": routeEntries(snapshot.Store, name), "strategy": routeStrategy(snapshot.Store)}}, nil
}

func routeStrategy(s *store.Store) string {
	if s.SingBox.DNS == nil || s.SingBox.DNS.Strategy == "" {
		return "asis"
	}
	return s.SingBox.DNS.Strategy
}

// routingChange also completes a configuration whose direct strategy was never
// chosen for this host: every routing change rewrites the sing-box
// configuration, so each is a point where it converges. The host's addresses
// are read first, outside the state lock.
func (a *App) routingChange(ctx context.Context, change func(*store.Store) (any, error)) (Result, error) {
	detected, _ := hostDirectStrategy(ctx)
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		// Resolved before the change, so a change that sets the strategy
		// itself has the last word and one that reports it reports the result.
		routing.ResolveDirectStrategy(snapshot.Store, detected)
		data, err := change(snapshot.Store)
		if err != nil {
			return Result{}, err
		}
		changed := snapshot.Store.IsDirty()
		if changed {
			routing.Sync(snapshot.Store)
			if err := a.Commit(ctx, snapshot); err != nil {
				return Result{}, err
			}
		}
		if err := a.Activate(ctx, snapshot, service.SingBox); err != nil {
			return Result{}, err
		}
		return Result{Changed: changed, Data: data}, nil
	})
}

// routingUserExists rejects a name no user has. It is invalid_argument rather
// than not_found because the name came from --user: the same answer `sub` and
// `user rename` give, so one missing user is one exit code across the CLI.
func routingUserExists(s *store.Store, name string) error {
	if !slices.Contains(derived.UserNames(s), name) {
		return Invalid("user not found")
	}
	return nil
}

func routeOutbound(s *store.Store, name string) (string, error) {
	for _, raw := range s.SingBox.Outbounds {
		header, err := store.ParseOutboundHeader(raw)
		if err != nil {
			return "", err
		}
		if header.Tag == name || (name == "direct" && header.Type == "direct") {
			return header.Tag, nil
		}
	}
	return "", Invalid("outbound not found")
}

func (a *App) RoutingSet(ctx context.Context, name string, presets []string, outbound string) (Result, error) {
	if len(presets) == 0 {
		return Result{}, Invalid("at least one preset is required")
	}
	selected := make([]routing.Preset, 0, len(presets))
	for _, name := range presets {
		preset, ok := routing.FindPreset(name)
		if !ok || name == "custom" {
			return Result{}, Invalid("unknown preset " + name)
		}
		selected = append(selected, preset)
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if err := routingUserExists(s, name); err != nil {
			return nil, err
		}
		target, err := routeOutbound(s, outbound)
		if err != nil {
			return nil, err
		}
		// add only creates. A preset the user already has a rule for is left
		// as it is, whatever outbound it uses: changing where it goes is what
		// modify is for, and add quietly repointing it was an edit by another
		// name.
		have := map[string]bool{}
		for _, entry := range routeEntries(s, name) {
			if entry.Preset != "" {
				have[entry.Preset] = true
			}
		}
		added, already := []string{}, []string{}
		for _, preset := range selected {
			if have[preset.Name] {
				already = append(already, preset.Name)
				continue
			}
			if err := routing.SetRule(s, name, routing.PresetToRule(preset, name, target)); err != nil {
				return nil, err
			}
			added = append(added, preset.Name)
		}
		// The user's rules after the change, so the reader sees where every
		// one of them now goes rather than only the ones just added.
		return map[string]any{"user": name, "presets": added, "already_added": already, "outbound": routing.OutboundLabel(target), "strategy": routeStrategy(s), "rules": routeEntries(s, name)}, nil
	})
}

// RoutingRules changes the rules --rules selects for one user. A selector is
// what `route rule list` prints beside a rule: a preset's menu index, or cN
// for the Nth rule no preset made. Every selector must name a rule
// the user has, or nothing changes.
func (a *App) RoutingRules(ctx context.Context, name string, selectors []string, outbound string, remove bool) (Result, error) {
	if len(selectors) == 0 {
		return Result{}, Invalid("at least one rule is required")
	}
	for _, selector := range selectors {
		if !routing.ValidRuleSelector(selector) {
			return Result{}, Invalid("unknown rule " + strings.TrimSpace(selector) + "; run gproxy route rule list")
		}
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if err := routingUserExists(s, name); err != nil {
			return nil, err
		}
		before := routeEntries(s, name)
		positions := map[string]int{}
		for _, entry := range before {
			positions[entry.Selector] = entry.Index
		}
		chosen := map[string]bool{}
		indexes := make([]int, 0, len(selectors))
		for _, selector := range selectors {
			key := strings.ToLower(strings.TrimSpace(selector))
			if preset, ok := routing.ResolvePresetSelector(key); ok {
				key = routing.PresetSymbol(preset)
			}
			position, ok := positions[key]
			if !ok {
				return nil, Invalid(name + " has no rule " + strings.TrimSpace(selector) + "; run gproxy route rule list --user " + name)
			}
			indexes = append(indexes, position)
			chosen[key] = true
		}
		var count int
		var err error
		if remove {
			count, err = routing.DeleteUserRulesByIndex(s, name, indexes)
		} else {
			target, targetErr := routeOutbound(s, outbound)
			if targetErr != nil {
				return nil, targetErr
			}
			count, err = routing.ReplaceUserRuleOutbounds(s, name, indexes, target)
		}
		data := map[string]any{"user": name, "affected_rules": count, "strategy": routeStrategy(s), "rules": routeEntries(s, name)}
		if remove {
			removed := []RouteEntry{}
			for _, entry := range before {
				if chosen[entry.Selector] {
					removed = append(removed, entry)
				}
			}
			data["removed"] = removed
		}
		return data, err
	})
}

// RoutingClear removes every rule for one user, or for all users when name is
// empty. all is the --all the caller actually passed: deriving it from an empty
// name reported "all": false on a request that was spelled --all.
func (a *App) RoutingClear(ctx context.Context, name string, all bool) (Result, error) {
	if !all {
		return Result{}, Invalid("select one user or --all")
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if name != "" {
			if err := routingUserExists(s, name); err != nil {
				return nil, err
			}
		}
		removed := routeEntries(s, name)
		count := 0
		if name == "" {
			count = routing.ClearAll(s)
		} else {
			count = routing.ClearUser(s, name)
		}
		return map[string]any{"user": name, "all": all, "affected_rules": count, "rules": []RouteEntry{}, "removed": removed}, nil
	})
}

// DirectStrategies lists the accepted direct egress strategies, shared by the
// validator below and shell completion.
func DirectStrategies() []string {
	return []string{"ipv4_only", "ipv6_only", "prefer_ipv4", "prefer_ipv6", "asis", "auto"}
}

// hostDirectStrategy reads the families this host actually has and names the
// strategy that matches: only IPv4 asks for A alone, only IPv6 asks for AAAA
// alone, and a dual-stack host prefers IPv4 while still allowing the other.
// Asking for a family the host cannot use is a lookup that can only return an
// address nothing can connect to.
//
// Loopback and link-local are not addresses this host is reached on, the same
// judgement the dashboard's network row makes.
func hostDirectStrategy(ctx context.Context) (string, error) {
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	observed, err := network.Observe(observationCtx)
	if err != nil {
		return "", err
	}
	var hasV4, hasV6 bool
	for _, address := range observed.Addresses {
		if address.Scope == "loopback" || address.Scope == "link_local" {
			continue
		}
		switch address.Family {
		case "ipv4":
			hasV4 = true
		case "ipv6":
			hasV6 = true
		}
	}
	switch {
	case hasV4 && hasV6:
		return "prefer_ipv4", nil
	case hasV4:
		return "ipv4_only", nil
	case hasV6:
		return "ipv6_only", nil
	}
	return "", Invalid("this host has no usable address to choose a strategy from")
}

func (a *App) RoutingDirect(ctx context.Context, strategy string, set bool) (Result, error) {
	if !set {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		return Result{Data: map[string]any{"strategy": routeStrategy(snapshot.Store)}}, nil
	}
	if !slices.Contains(DirectStrategies(), strategy) {
		return Result{}, Invalid("unsupported direct strategy")
	}
	// Resolved before the state lock: it reads the host's interfaces.
	detected := ""
	if strategy == "auto" {
		chosen, err := hostDirectStrategy(ctx)
		if err != nil {
			return Result{}, err
		}
		strategy, detected = chosen, chosen
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if routeStrategy(s) != strategy || !routing.DirectStrategyResolved(s) {
			value := strategy
			if value == "asis" {
				value = ""
			}
			routing.ApplyDirectStrategy(s, value)
		}
		data := map[string]any{"strategy": strategy}
		if detected != "" {
			data["detected"] = detected
		}
		return data, nil
	})
}

// RoutingSyncDNS recompiles the route and DNS rules from the stored user rules
// without changing any of them. Every other routing operation already
// recompiles as part of its own change, so this is the way to pick up a change
// in how rules compile -- a new gproxy deciding a chain's resolver families
// differently, say -- without inventing a state change to trigger it.
//
// It reports changed only when the recompiled configuration actually differs
// from the one on disk. Recompiling an already-correct configuration produces
// the same bytes, and saying "changed" there tells a caller something happened
// when nothing did.
//
// Like every routing change it also chooses the direct strategy for a
// configuration that never had one, which makes it the command that upgrades
// an existing installation without changing anything else.
func (a *App) RoutingSyncDNS(ctx context.Context) (Result, error) {
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		s.MarkDirty(store.FileSingBox)
		routing.Sync(s)
		s.Settled(store.FileSingBox)
		return map[string]any{"strategy": routeStrategy(s)}, nil
	})
}

type ChainView struct {
	Tag  string `json:"tag"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// Address is Host and Port already joined, so an IPv6 literal is bracketed
	// once here rather than in every caller that wants to print it.
	Address string `json:"address"`
	// DomainStrategy is the address-family preference this chain's lookups use,
	// taken from what its endpoint supports.
	DomainStrategy string `json:"domain_strategy,omitempty"`
	// Resolver is the DNS server this chain's lookups go to, reached through
	// the chain itself.
	Resolver      string `json:"resolver,omitempty"`
	Authenticated bool   `json:"authenticated"`
	// Users are the users whose rules select this chain. It is the same
	// relationship the rule listing shows, read from the other end, and it is
	// what makes a refused removal predictable.
	Users []string `json:"users"`
	// Final is true when unmatched traffic leaves through this chain.
	Final bool `json:"final,omitempty"`
}

// resolveVia pins a resolver hostname to an address the chain can actually
// reach. The lookup follows the chain's own strategy: a resolver reached
// through an IPv4 endpoint has to be an IPv4 address, or every lookup for that
// chain is sent to somewhere the chain cannot dial. The other family is only
// tried when the preferred one has no answer.
func (a *App) resolveVia(ctx context.Context, name, strategy string) (net.IP, error) {
	lookupCtx, cancel := ObservationContext(ctx)
	defer cancel()
	order := []string{"ip"}
	switch {
	case strings.Contains(strategy, "ipv4"):
		order = []string{"ip4", "ip6"}
	case strings.Contains(strategy, "ipv6"):
		order = []string{"ip6", "ip4"}
	}
	var lastErr error
	for _, network := range order {
		addresses, err := net.DefaultResolver.LookupIP(lookupCtx, network, name)
		if err != nil {
			lastErr = err
			continue
		}
		if len(addresses) > 0 {
			return addresses[0], nil
		}
	}
	return nil, lastErr
}

// chainStrategy decides which address families a chain's lookups ask for. An
// IP endpoint answers for itself and asks for that family alone. A hostname is
// resolved, bounded and best-effort: one that answers in a single family asks
// for that family alone, one that answers in both can carry either and so
// prefers IPv4, and one that does not resolve is left on the configured
// strategy rather than guessed at.
func chainStrategy(ctx context.Context, host string) string {
	if known := routing.ChainStrategy(host); known != "" {
		return known
	}
	lookupCtx, cancel := ObservationContext(ctx)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIP(lookupCtx, "ip", host)
	if err != nil {
		return ""
	}
	var hasV4, hasV6 bool
	for _, address := range addresses {
		if address.To4() != nil {
			hasV4 = true
		} else {
			hasV6 = true
		}
	}
	switch {
	case hasV4 && hasV6:
		return "prefer_ipv4"
	case hasV4:
		return "ipv4_only"
	case hasV6:
		return "ipv6_only"
	}
	return ""
}

func chainAddress(chain routing.ChainOutbound) string {
	return net.JoinHostPort(chain.Server, strconv.Itoa(chain.ServerPort))
}

func chainView(chain routing.ChainOutbound) ChainView {
	return ChainView{
		Tag:           chain.Tag,
		Host:          chain.Server,
		Port:          chain.ServerPort,
		Address:       chainAddress(chain),
		Authenticated: chain.Username != "",
		Users:         []string{},
	}
}

func (a *App) RoutingChains(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	users := map[string][]string{}
	for _, rule := range snapshot.Store.UserRoutes {
		for _, user := range rule.AuthUser {
			if !slices.Contains(users[rule.Outbound], user) {
				users[rule.Outbound] = append(users[rule.Outbound], user)
			}
		}
	}
	views := []ChainView{}
	strategies := routing.ChainStrategies(snapshot.Store)
	resolvers := routing.ChainResolvers(snapshot.Store)
	for _, chain := range routing.ListChains(snapshot.Store) {
		view := chainView(chain)
		view.DomainStrategy = strategies[chain.Tag]
		view.Resolver = resolvers[chain.Tag]
		view.Final = routing.RouteFinal(snapshot.Store) == chain.Tag
		if referencing := users[chain.Tag]; len(referencing) > 0 {
			slices.Sort(referencing)
			view.Users = referencing
		}
		views = append(views, view)
	}
	return Result{Data: map[string]any{"chains": views}}, nil
}

// validateChainHost accepts an IP literal or a hostname, shared by add and
// modify so one spelling of a host cannot be accepted by only one of them.
func validateChainHost(host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /:@\t\r\n") {
		return Invalid("invalid chain host")
	}
	label := regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	for _, part := range strings.Split(host, ".") {
		if !label.MatchString(part) {
			return Invalid("invalid chain host")
		}
	}
	return nil
}

func (a *App) RoutingChainAdd(ctx context.Context, tag, host string, port int, username, password, resolver string) (Result, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`).MatchString(tag) || tag == "direct" {
		return Result{}, Invalid("invalid chain tag")
	}
	if port < 1 || port > 65535 {
		return Result{}, Invalid("port must be between 1 and 65535")
	}
	if err := validateChainHost(host); err != nil {
		return Result{}, err
	}
	if (username == "") != (password == "") {
		return Result{}, Invalid("chain credentials require both username and password")
	}
	// Decided before the state lock: both may need a lookup, and the compile
	// path that writes the DNS rules has no network of its own.
	strategy := chainStrategy(ctx, host)
	// No --dns means the default resolver, at the address family the chain
	// reaches: an IPv6-only chain cannot carry a query to 8.8.8.8.
	parsed := routing.DefaultChainResolverFor(strategy)
	if resolver != "" {
		var err error
		parsed, err = routing.ParseChainResolver(resolver, func(name string) (net.IP, error) {
			return a.resolveVia(ctx, name, strategy)
		})
		if err != nil {
			return Result{}, Invalid(err.Error())
		}
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		desired := routing.ChainOutbound{Type: "socks", Tag: tag, Server: host, ServerPort: port, Username: username, Password: password, Version: "5"}
		for _, raw := range s.SingBox.Outbounds {
			header, err := store.ParseOutboundHeader(raw)
			if err != nil {
				return nil, err
			}
			if header.Tag != tag {
				continue
			}
			var existing routing.ChainOutbound
			if json.Unmarshal(raw, &existing) == nil && existing == desired {
				return chainView(existing), nil
			}
			return nil, &Error{Code: "conflict", Message: "outbound tag already exists with different settings"}
		}
		if err := routing.AddChain(s, tag, host, port, username, password); err != nil {
			return nil, err
		}
		routing.SetChainDNS(s, tag, parsed, strategy)
		view := chainView(desired)
		view.DomainStrategy = strategy
		view.Resolver = routing.ChainResolvers(s)[tag]
		return view, nil
	})
}

// ChainChange is the set of fields a chain modification can carry. A zero field
// is one the caller did not name and this leaves alone, except Credentials,
// which is authoritative when SetCredentials is true because empty credentials
// are themselves a value.
type ChainChange struct {
	Host           string
	Port           int
	Username       string
	Password       string
	SetCredentials bool
	Resolver       string
}

// RoutingChainModify changes where an existing chain points without touching
// the rules that select it. Removing and re-adding was the alternative, and a
// chain a rule selects cannot be removed, so changing an upstream address meant
// pointing every rule elsewhere and back again.
func (a *App) RoutingChainModify(ctx context.Context, tag string, change ChainChange) (Result, error) {
	if change.Host == "" && change.Port == 0 && !change.SetCredentials && change.Resolver == "" {
		return Result{}, Invalid("select at least one of --parameter or --dns")
	}
	if change.Port != 0 && (change.Port < 1 || change.Port > 65535) {
		return Result{}, Invalid("port must be between 1 and 65535")
	}
	if change.Host != "" {
		if err := validateChainHost(change.Host); err != nil {
			return Result{}, err
		}
	}
	if change.SetCredentials && (change.Username == "") != (change.Password == "") {
		return Result{}, Invalid("chain credentials require both username and password")
	}
	// The current chain is read before the lock so the lookups below, which
	// need the network, happen outside it -- the same order `add` uses.
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	var current *routing.ChainOutbound
	for _, chain := range routing.ListChains(snapshot.Store) {
		if chain.Tag == tag {
			c := chain
			current = &c
		}
	}
	if current == nil {
		return Result{}, Invalid("chain not found")
	}
	desired := *current
	if change.Host != "" {
		desired.Server = change.Host
	}
	if change.Port != 0 {
		desired.ServerPort = change.Port
	}
	if change.SetCredentials {
		desired.Username, desired.Password = change.Username, change.Password
	}
	// The endpoint decides which families its lookups ask for, so a changed
	// host re-decides it.
	strategy := chainStrategy(ctx, desired.Server)
	resolver, known := routing.ChainResolverOf(snapshot.Store, tag)
	switch {
	case change.Resolver != "":
		resolver, err = routing.ParseChainResolver(change.Resolver, func(name string) (net.IP, error) {
			return a.resolveVia(ctx, name, strategy)
		})
		if err != nil {
			return Result{}, Invalid(err.Error())
		}
	case !known || routing.IsDefaultChainResolver(resolver):
		// The default follows the endpoint: moving a chain to an IPv6-only
		// address moves its default resolver there too.
		resolver = routing.DefaultChainResolverFor(strategy)
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if err := routing.ReplaceChain(s, desired); err != nil {
			return nil, Invalid(err.Error())
		}
		routing.SetChainDNS(s, tag, resolver, strategy)
		view := chainView(desired)
		view.DomainStrategy = strategy
		view.Resolver = routing.ChainResolvers(s)[tag]
		for _, rule := range s.UserRoutes {
			if rule.Outbound == tag {
				for _, user := range rule.AuthUser {
					if !slices.Contains(view.Users, user) {
						view.Users = append(view.Users, user)
					}
				}
			}
		}
		slices.Sort(view.Users)
		return view, nil
	})
}

func (a *App) RoutingChainRemove(ctx context.Context, tag string) (Result, error) {
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		found := false
		for _, chain := range routing.ListChains(s) {
			if chain.Tag == tag {
				found = true
				break
			}
		}
		if !found {
			return map[string]any{"tag": tag, "removed": false}, nil
		}
		// A refusal names what still uses the chain, every user's rules and
		// the route final together, so one answer is enough to clear it.
		referenced := slices.ContainsFunc(s.UserRoutes, func(rule store.UserRouteRule) bool { return rule.Outbound == tag })
		final := routing.RouteFinal(s) == tag
		if referenced || final {
			using := []RouteEntry{}
			for _, entry := range routeEntries(s, "") {
				if entry.Outbound == routing.OutboundLabel(tag) {
					using = append(using, entry)
				}
			}
			message := "chain is the route final; run gproxy route final set direct first"
			if referenced {
				message = "chain is referenced by user routes; remove or modify those rules first"
			}
			return nil, &Error{Code: "conflict", Message: message, Data: map[string]any{"tag": tag, "rules": using, "route_final": final}}
		}
		if err := routing.RemoveChain(s, tag); err != nil {
			return nil, err
		}
		return map[string]any{"tag": tag, "removed": true}, nil
	})
}

func (a *App) RoutingTest(ctx context.Context, name, domain string) (Result, error) {
	if domain == "" || strings.ContainsAny(domain, " /\t\r\n") {
		return Result{}, Invalid("expected a domain or ip address")
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := routingUserExists(snapshot.Store, name); err != nil {
		return Result{}, err
	}
	target := strings.TrimSuffix(strings.ToLower(domain), ".")
	sets, err := loadRuleSetFiles(snapshot.Store)
	if err != nil {
		return Result{}, err
	}
	defer sets.Close()
	result, err := routing.Evaluate(snapshot.Store, name, target, sets.matcher(ctx, target))
	if err != nil {
		return Result{}, err
	}
	data := map[string]any{"user": name, "domain": domain, "evaluation": result, "connectivity_checked": false}
	if entry, ok := decidingRule(snapshot.Store, name, result.Decision); ok {
		data["rule"] = entry
	}
	for _, chain := range routing.ListChains(snapshot.Store) {
		if chain.Tag == result.Decision.Outbound {
			data["address"] = chainAddress(chain)
		}
	}
	return Result{Data: data}, nil
}

// decidingRule is the user's own rule behind the compiled one that decided,
// numbered as route rule list numbers it. Compiled rules are merged across
// users, so the match is by what matched: a preset holding that rule set, or
// a rule carrying that domain or address.
func decidingRule(s *store.Store, name string, decision routing.Decision) (RouteEntry, bool) {
	if decision.Rule < 0 {
		return RouteEntry{}, false
	}
	for _, entry := range routeEntries(s, name) {
		if entry.Outbound != decision.Outbound {
			continue
		}
		rule := entry.Rule
		var items []string
		switch decision.MatchBy {
		case "rule_set":
			items = rule.RuleSet
			if preset, ok := routing.FindPreset(entry.Preset); ok {
				items = append(slices.Clone(items), preset.RuleSets...)
			}
		case "domain":
			items = rule.Domain
		case "domain_suffix":
			items = rule.DomainSuffix
		case "domain_keyword":
			items = rule.DomainKeyword
		case "domain_regex":
			items = rule.DomainRegex
		case "ip_cidr":
			items = rule.IPCIDR
		case "auth_user":
			return entry, true
		}
		if slices.Contains(items, decision.Value) {
			return entry, true
		}
	}
	return RouteEntry{}, false
}

// RoutingFinal reads or sets where connections no rule claims leave: direct,
// or a chain for shell-proxy's whole-server chain mode. Setting a chain also
// moves the DNS final to that chain's resolver, which the sync that follows
// the change writes.
func (a *App) RoutingFinal(ctx context.Context, target string, set bool) (Result, error) {
	if !set {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		return Result{Data: finalView(snapshot.Store)}, nil
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		tag, err := routeOutbound(s, target)
		if err != nil {
			return nil, Invalid("unknown outbound " + target + "; use direct or a chain tag from gproxy route chain list")
		}
		if s.SingBox.Route == nil {
			s.SingBox.Route = &store.RouteConfig{}
		}
		if s.SingBox.Route.Final != tag {
			s.SingBox.Route.Final = tag
			s.MarkDirty(store.FileSingBox)
		}
		return finalView(s), nil
	})
}

func finalView(s *store.Store) map[string]any {
	final := routing.RouteFinal(s)
	view := map[string]any{"final": final, "dns_final": routing.DNSFinal(s)}
	for _, chain := range routing.ListChains(s) {
		if chain.Tag == final {
			view["address"] = chainAddress(chain)
		}
	}
	return view
}
