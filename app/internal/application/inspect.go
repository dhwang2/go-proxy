package application

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"go-proxy/internal/cert"
	"go-proxy/internal/derived"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/pkg/sysutil"
)

func (a *App) Status(ctx context.Context, probe bool) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	states, stateErr := service.Snapshot(observationCtx)
	netInfo, netErr := network.Observe(observationCtx, probe)
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
		"system": map[string]any{"os": runtime.GOOS, "arch": sysutil.Arch()}, "cert": cert.Inspect(),
		"users": len(derived.UserNames(snapshot.Store)), "nodes": len(derived.Inventory(snapshot.Store)), "routing_rules": len(snapshot.Store.UserRoutes),
	}}, nil
}

func (a *App) ConfigView(ctx context.Context, kind string, secrets bool) (Result, error) {
	if kind != "sing-box" && kind != "snell" && kind != "shadow-tls" {
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
		value = map[string]any{"listen": conf.Listen, "psk": conf.PSK, "ipv6": conf.IPv6}
	case "shadow-tls":
		bindings := make([]map[string]any, 0, len(snapshot.Bindings))
		for _, b := range snapshot.Bindings {
			bindings = append(bindings, map[string]any{"listen_port": b.ListenPort, "backend_port": b.BackendPort, "backend_protocol": b.BackendProto, "password": b.Password, "sni": b.SNI, "version": b.Version})
		}
		value = bindings
	}
	if !secrets {
		redactConfig(value)
	}
	return Result{Data: map[string]any{"component": kind, "configuration": value, "secrets_included": secrets}}, nil
}

func redactSingBox(ctx context.Context, conf *store.SingBoxConfig) error {
	for i := range conf.Inbounds {
		if err := ctx.Err(); err != nil {
			return err
		}
		ib := &conf.Inbounds[i]
		if ib.Password != "" {
			ib.Password = "<redacted>"
		}
		for j := range ib.Users {
			if ib.Users[j].Password != "" {
				ib.Users[j].Password = "<redacted>"
			}
			if ib.Users[j].UUID != "" {
				ib.Users[j].UUID = "<redacted>"
			}
		}
		if ib.TLS != nil && ib.TLS.Reality != nil && ib.TLS.Reality.PrivateKey != "" {
			ib.TLS.Reality.PrivateKey = "<redacted>"
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
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			redactConfig(value)
			encoded, err := json.Marshal(value)
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

func redactConfig(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch strings.ToLower(key) {
			case "password", "psk", "uuid", "private_key", "key":
				v[key] = "<redacted>"
			default:
				redactConfig(child)
			}
		}
	case []any:
		for _, child := range v {
			redactConfig(child)
		}
	case []map[string]any:
		for _, child := range v {
			redactConfig(child)
		}
	}
}

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
		matched := false
		switch binding.BackendProto {
		case "ss":
			for _, inbound := range snapshot.Store.SingBox.Inbounds {
				if inbound.Type == "shadowsocks" && inbound.ListenPort == binding.BackendPort {
					matched = true
					break
				}
			}
		case "snell":
			matched = snapshot.Store.SnellConf != nil && snapshot.Store.SnellConf.Port() == binding.BackendPort
		}
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
