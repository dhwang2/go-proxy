package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"

	"go-proxy/internal/derived"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

type RouteEntry struct {
	User  string              `json:"user"`
	Index int                 `json:"index"`
	Label string              `json:"label"`
	Rule  store.UserRouteRule `json:"rule"`
}

func (a *App) RoutingList(ctx context.Context, name string) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	if name != "" && !slices.Contains(derived.UserNames(snapshot.Store), name) {
		return Result{}, &Error{Code: "not_found", Message: "user not found"}
	}
	entries := []RouteEntry{}
	counts := map[string]int{}
	for _, rule := range snapshot.Store.UserRoutes {
		for _, user := range rule.AuthUser {
			if name != "" && user != name {
				continue
			}
			counts[user]++
			entries = append(entries, RouteEntry{User: user, Index: counts[user], Label: routing.UserRouteLabel(rule), Rule: rule})
		}
	}
	return Result{Data: map[string]any{"rules": entries, "strategy": routeStrategy(snapshot.Store)}}, nil
}

// RoutingOverview answers "what is my routing" in one command-scoped snapshot.
// Composing it from RoutingList, RoutingChains and RoutingDirect would take the
// state lock three times and could report three different instants.
func (a *App) RoutingOverview(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	entries := []RouteEntry{}
	counts := map[string]int{}
	for _, rule := range snapshot.Store.UserRoutes {
		for _, user := range rule.AuthUser {
			counts[user]++
			entries = append(entries, RouteEntry{User: user, Index: counts[user], Label: routing.UserRouteLabel(rule), Rule: rule})
		}
	}
	views := []ChainView{}
	for _, chain := range routing.ListChains(snapshot.Store) {
		views = append(views, chainView(chain))
	}
	return Result{Data: map[string]any{"rules": entries, "chains": views, "strategy": routeStrategy(snapshot.Store)}}, nil
}

func routeStrategy(s *store.Store) string {
	if s.SingBox.DNS == nil || s.SingBox.DNS.Strategy == "" {
		return "asis"
	}
	return s.SingBox.DNS.Strategy
}

func (a *App) routingChange(ctx context.Context, change func(*store.Store) (any, error)) (Result, error) {
	return a.Operation(ctx, func() (Result, error) {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		data, err := change(snapshot.Store)
		if err != nil {
			return Result{}, err
		}
		changed := snapshot.Store.IsDirty()
		if changed {
			routing.Sync(snapshot.Store)
			a.Progress("saving routing configuration")
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

func routingUserExists(s *store.Store, name string) error {
	if !slices.Contains(derived.UserNames(s), name) {
		return &Error{Code: "not_found", Message: "user not found"}
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
	return "", &Error{Code: "not_found", Message: "outbound not found"}
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
		for _, preset := range selected {
			if err := routing.SetRule(s, name, routing.PresetToRule(preset, name, target)); err != nil {
				return nil, err
			}
		}
		return map[string]any{"user": name, "presets": presets, "outbound": target, "strategy": routeStrategy(s)}, nil
	})
}

func (a *App) RoutingRules(ctx context.Context, name string, indexes []int, outbound string, remove bool) (Result, error) {
	if len(indexes) == 0 {
		return Result{}, Invalid("at least one rule index is required")
	}
	for _, index := range indexes {
		if index < 1 {
			return Result{}, Invalid("rule indexes start at 1")
		}
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if err := routingUserExists(s, name); err != nil {
			return nil, err
		}
		total := 0
		for _, rule := range s.UserRoutes {
			if slices.Contains(rule.AuthUser, name) {
				total++
			}
		}
		for _, index := range indexes {
			if index > total {
				return nil, Invalid(fmt.Sprintf("rule index %d exceeds the current user rule count", index))
			}
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
		return map[string]any{"user": name, "affected_rules": count, "strategy": routeStrategy(s)}, err
	})
}

func (a *App) RoutingClear(ctx context.Context, name string, all bool) (Result, error) {
	if all == (name != "") {
		return Result{}, Invalid("select one user or --all")
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		count := 0
		if all {
			count = routing.ClearAll(s)
		} else {
			if err := routingUserExists(s, name); err != nil {
				return nil, err
			}
			count = routing.ClearUser(s, name)
		}
		return map[string]any{"user": name, "all": all, "affected_rules": count}, nil
	})
}

func (a *App) RoutingDirect(ctx context.Context, strategy string, set bool) (Result, error) {
	if !set {
		snapshot, err := a.Snapshot(ctx)
		if err != nil {
			return Result{}, err
		}
		return Result{Data: map[string]any{"strategy": routeStrategy(snapshot.Store)}}, nil
	}
	if !slices.Contains([]string{"ipv4_only", "ipv6_only", "prefer_ipv4", "prefer_ipv6", "asis"}, strategy) {
		return Result{}, Invalid("unsupported direct strategy")
	}
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		if routeStrategy(s) != strategy {
			value := strategy
			if value == "asis" {
				value = ""
			}
			if s.SingBox.DNS == nil {
				s.SingBox.DNS = &store.DNSConfig{}
			}
			s.SingBox.DNS.Strategy = value
			s.MarkDirty(store.FileSingBox)
		}
		return map[string]any{"strategy": strategy}, nil
	})
}

func (a *App) RoutingSyncDNS(ctx context.Context) (Result, error) {
	return a.routingChange(ctx, func(s *store.Store) (any, error) {
		s.MarkDirty(store.FileSingBox)
		return map[string]any{"strategy": routeStrategy(s)}, nil
	})
}

type ChainView struct {
	Tag           string `json:"tag"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Authenticated bool   `json:"authenticated"`
}

func chainView(chain routing.ChainOutbound) ChainView {
	return ChainView{Tag: chain.Tag, Host: chain.Server, Port: chain.ServerPort, Authenticated: chain.Username != ""}
}

func (a *App) RoutingChains(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	views := []ChainView{}
	for _, chain := range routing.ListChains(snapshot.Store) {
		views = append(views, chainView(chain))
	}
	return Result{Data: map[string]any{"chains": views}}, nil
}

func (a *App) RoutingChainAdd(ctx context.Context, tag, host string, port int, username, password string) (Result, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`).MatchString(tag) || tag == "direct" {
		return Result{}, Invalid("invalid chain tag")
	}
	if port < 1 || port > 65535 {
		return Result{}, Invalid("port must be between 1 and 65535")
	}
	if net.ParseIP(host) == nil {
		if len(host) > 253 || strings.ContainsAny(host, " /:@\t\r\n") || host == "" {
			return Result{}, Invalid("invalid chain host")
		}
		for _, label := range strings.Split(host, ".") {
			if !regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`).MatchString(label) {
				return Result{}, Invalid("invalid chain host")
			}
		}
	}
	if (username == "") != (password == "") {
		return Result{}, Invalid("chain credentials require both username and password")
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
		return chainView(desired), nil
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
			return map[string]any{"tag": tag}, nil
		}
		for _, rule := range s.UserRoutes {
			if rule.Outbound == tag {
				return nil, &Error{Code: "conflict", Message: "chain is referenced by user routes; remove or modify those rules first"}
			}
		}
		if err := routing.RemoveChain(s, tag); err != nil {
			return nil, err
		}
		return map[string]any{"tag": tag}, nil
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
	result := routing.TestDomain(snapshot.Store, name, strings.ToLower(domain))
	return Result{Data: map[string]any{"user": name, "domain": domain, "evaluation": result, "connectivity_checked": false}}, nil
}
