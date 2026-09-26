package application

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"go-proxy/internal/cert"
	"go-proxy/internal/derived"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/pkg/jsonorder"
	"go-proxy/pkg/sysutil"
)

func (a *App) Status(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	netInfo, netErr := network.Observe(observationCtx)
	// A family whose interfaces carry only private addresses is behind NAT,
	// and its address as clients see it can only be learned from outside.
	// That lookup runs on every status, by the operator's choice, alongside
	// the service query and inside the same deadline. A family with a public
	// interface address, or none at all, makes no call.
	probes := map[string]chan network.Probe{}
	if netErr == nil {
		for _, family := range []string{"ipv4", "ipv6"} {
			if netInfo.HasFamily(family) && !netInfo.HasGlobal(family) {
				result := make(chan network.Probe, 1)
				probes[family] = result
				go func(family string) { result <- network.ProbeFamily(observationCtx, family) }(family)
			}
		}
	}
	states, stateErr := service.Snapshot(observationCtx)
	if result, ok := probes["ipv4"]; ok {
		netInfo.IPv4 = <-result
	}
	if result, ok := probes["ipv6"]; ok {
		netInfo.IPv6 = <-result
	}
	issues := []string{}
	if stateErr != nil {
		issues = append(issues, stateErr.Error())
	}
	if netErr != nil {
		issues = append(issues, netErr.Error())
	}
	healthy := stateErr == nil
	expected := map[service.Name]bool{
		service.SingBox:   len(snapshot.Store.SingBox.Inbounds) > 0,
		service.Snell:     snapshot.Store.SnellConf != nil,
		service.ShadowTLS: len(snapshot.Bindings) > 0,
		service.CaddySub:  cert.ReadDomain() != "",
		service.Watchdog:  true,
	}
	for _, state := range states {
		if expected[state.Name] && (!state.Installed || !state.Running) {
			healthy = false
			if !state.Installed {
				issues = append(issues, fmt.Sprintf("configured service %s is missing", state.Name))
			} else {
				issues = append(issues, fmt.Sprintf("configured service %s is %s", state.Name, state.State))
			}
		}
	}
	return Result{Data: map[string]any{
		"healthy": healthy, "complete": stateErr == nil && netErr == nil && netInfo.Complete,
		"services": states, "network": netInfo, "issues": issues,
		"system": map[string]any{"os": runtime.GOOS, "arch": sysutil.Arch(), "version": osVersion()},
		"cert":   cert.Inspect(), "memberships": membershipsByUser(snapshot),
		"users": len(derived.UserNames(snapshot.Store)), "nodes": len(derived.Inventory(snapshot.Store)), "routing_rules": len(snapshot.Store.UserRoutes),
	}}, nil
}

// ConfigKinds lists the inspectable configuration sources. Completion and the
// validator below read the same slice so they cannot drift apart.
func ConfigKinds() []string { return []string{"sing-box", "snell", "shadow-tls"} }

// membershipsByUser inverts the node list so the dashboard can show what each
// user actually has, which is the question "2 users, 5 nodes" never answered.
// Protocol types are deduplicated per user: two SS nodes read as one capability.
func membershipsByUser(snapshot *Snapshot) map[string][]string {
	byUser := map[string][]string{}
	for _, name := range derived.UserNames(snapshot.Store) {
		byUser[name] = []string{}
	}
	for _, node := range protocolNodes(snapshot) {
		for _, user := range node.Users {
			if user == "" {
				continue
			}
			if slices.Contains(byUser[user], node.Type) {
				continue
			}
			byUser[user] = append(byUser[user], node.Type)
		}
	}
	for _, types := range byUser {
		slices.Sort(types)
	}
	return byUser
}

func (a *App) ConfigView(ctx context.Context, kind string, secrets bool) (Result, error) {
	if !slices.Contains(ConfigKinds(), kind) {
		return Result{}, Invalid("configuration must be sing-box, snell or shadow-tls")
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	var value any
	switch kind {
	case "sing-box":
		if !secrets {
			if err := redactSingBox(ctx, snapshot.Store.SingBox); err != nil {
				return Result{}, err
			}
		}
		value = snapshot.Store.SingBox
	case "snell":
		conf := snapshot.Store.SnellConf
		if conf == nil {
			return Result{}, &Error{Code: "not_found", Message: "snell is not configured"}
		}
		view := snellView{Listen: conf.Listen, PSK: conf.PSK, Mode: conf.Mode, DNSIPPreference: conf.DNSIPPreference, DNS: conf.DNS, EgressInterface: conf.EgressInterface}
		if !secrets {
			view.PSK = redactedValue
		}
		value = view
	case "shadow-tls":
		bindings := make([]shadowTLSView, 0, len(snapshot.Bindings))
		for _, b := range snapshot.Bindings {
			view := shadowTLSView{ListenPort: b.ListenPort, BackendProtocol: b.BackendProto, BackendPort: b.BackendPort, SNI: b.SNI, Password: b.Password, Version: b.Version}
			if !secrets {
				view.Password = redactedValue
			}
			bindings = append(bindings, view)
		}
		value = bindings
	}
	return Result{Data: map[string]any{"component": kind, "configuration": value, "secrets_included": secrets}}, nil
}

const redactedValue = "<redacted>"

// The snell and shadow-tls views are structs rather than maps so they print in
// the order a reader follows them: where it listens, then what it needs.
type snellView struct {
	Listen          string `json:"listen"`
	PSK             string `json:"psk"`
	Mode            string `json:"mode"`
	DNSIPPreference string `json:"dns_ip_preference"`
	DNS             string `json:"dns,omitempty"`
	EgressInterface string `json:"egress_interface,omitempty"`
}

type shadowTLSView struct {
	ListenPort      int    `json:"listen_port"`
	BackendProtocol string `json:"backend_protocol"`
	BackendPort     int    `json:"backend_port"`
	SNI             string `json:"sni"`
	Password        string `json:"password"`
	Version         int    `json:"version"`
}

func redactSingBox(ctx context.Context, conf *store.SingBoxConfig) error {
	for i := range conf.Inbounds {
		if err := ctx.Err(); err != nil {
			return err
		}
		ib := &conf.Inbounds[i]
		for j := range ib.Users {
			if ib.Users[j].Password != "" {
				ib.Users[j].Password = redactedValue
			}
			if ib.Users[j].UUID != "" {
				ib.Users[j].UUID = redactedValue
			}
		}
		if ib.TLS != nil && ib.TLS.Reality != nil && ib.TLS.Reality.PrivateKey != "" {
			ib.TLS.Reality.PrivateKey = redactedValue
		}
	}
	groups := [][]json.RawMessage{conf.Outbounds}
	if conf.DNS != nil {
		groups = append(groups, conf.DNS.Servers)
	}
	if conf.Route != nil {
		groups = append(groups, conf.Route.RuleSet)
	}
	if len(conf.Experimental) > 0 {
		groups = append(groups, []json.RawMessage{conf.Experimental})
	}
	for _, group := range groups {
		for i, raw := range group {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Parsed with its key order kept: through a map, every entry
			// read back alphabetically and no longer matched the file.
			value, err := jsonorder.Parse(raw)
			if err != nil {
				return err
			}
			redactOrdered(value)
			encoded, err := value.MarshalJSON()
			if err != nil {
				return err
			}
			group[i] = encoded
		}
	}
	if len(conf.Experimental) > 0 {
		conf.Experimental = groups[len(groups)-1][0]
	}
	return nil
}

func redactOrdered(value *jsonorder.Value) {
	switch value.Kind {
	case jsonorder.Object:
		for index, key := range value.Keys {
			if slices.Contains(secretKeys, strings.ToLower(key)) {
				value.Fields[index] = jsonorder.String(redactedValue)
				continue
			}
			redactOrdered(value.Fields[index])
		}
	case jsonorder.Array:
		for _, item := range value.Items {
			redactOrdered(item)
		}
	}
}

var secretKeys = []string{"password", "psk", "uuid", "private_key", "key"}

func (a *App) ConfigValidate(ctx context.Context) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	return validateConfiguration(ctx, snapshot)
}

func validateConfiguration(ctx context.Context, snapshot *Snapshot) (Result, error) {
	if err := snapshot.Store.Validate(ctx); err != nil {
		return Result{}, &Error{Code: "validation_failed", Message: err.Error()}
	}
	checks := []map[string]string{{"component": "sing-box", "validation": "core", "state": "passed"}}
	if conf := snapshot.Store.SnellConf; conf != nil {
		if conf.Port() < 1 || conf.Port() > 65535 || len(conf.PSK) < 12 || len(conf.PSK) > 255 {
			return Result{}, &Error{Code: "validation_failed", Message: "invalid snell listener or credential length"}
		}
		if mode, preference := conf.Settings(); !slices.Contains(store.SnellModes, mode) || !slices.Contains(store.SnellDNSIPPreferences, preference) {
			return Result{}, &Error{Code: "validation_failed", Message: "invalid snell mode or dns-ip-preference"}
		}
		if !service.BinaryInstalled(service.Snell) {
			return Result{}, &Error{Code: "validation_unavailable", Message: "snell core is not installed"}
		}
		checks = append(checks, map[string]string{"component": "snell", "validation": "configuration schema", "state": "passed"})
	}
	ports := make(map[int]bool)
	for _, inbound := range snapshot.Store.SingBox.Inbounds {
		ports[inbound.ListenPort] = true
	}
	if snapshot.Store.SnellConf != nil {
		ports[snapshot.Store.SnellConf.Port()] = true
	}
	for _, binding := range snapshot.Bindings {
		if binding.ListenPort < 1 || binding.ListenPort > 65535 || binding.BackendPort < 1 || binding.BackendPort > 65535 || binding.Password == "" || !cert.IsValidDomain(binding.SNI) || binding.Version != 3 {
			return Result{}, &Error{Code: "validation_failed", Message: fmt.Sprintf("invalid shadow-tls binding on port %d", binding.ListenPort)}
		}
		// Snell is the only backend a shadow-tls listener can front.
		matched := binding.BackendProto == "snell" &&
			snapshot.Store.SnellConf != nil && snapshot.Store.SnellConf.Port() == binding.BackendPort
		if !matched {
			return Result{}, &Error{Code: "validation_failed", Message: fmt.Sprintf("shadow-tls binding on port %d has no matching backend", binding.ListenPort)}
		}
		if ports[binding.ListenPort] {
			return Result{}, &Error{Code: "validation_failed", Message: fmt.Sprintf("shadow-tls listener port %d conflicts with another listener", binding.ListenPort)}
		}
		ports[binding.ListenPort] = true
	}
	if len(snapshot.Bindings) > 0 {
		if !service.BinaryInstalled(service.ShadowTLS) {
			return Result{}, &Error{Code: "validation_unavailable", Message: "shadow-tls core is not installed"}
		}
		checks = append(checks, map[string]string{"component": "shadow-tls", "validation": "binding schema", "state": "passed"})
	}
	return Result{Data: map[string]any{"valid": true, "checks": checks}}, nil
}
