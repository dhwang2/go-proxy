package application

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"go-proxy/internal/cert"
	"go-proxy/internal/config"
	"go-proxy/internal/core"
	"go-proxy/internal/logs"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
	"go-proxy/internal/update"
	"go-proxy/pkg/fileutil"
)

const lockSetupRules = "d " + config.LockDir + " 0755 root root -\nf " + config.LockDir + "/.state.lock 0600 root root -\n"

func (a *App) Init(ctx context.Context) (Result, error) {
	return a.Operation(ctx, func() (Result, error) {
		_, before := os.Stat(config.SingBoxConfig)
		a.Progress("initializing runtime")
		changed := os.IsNotExist(before)
		if err := a.State(ctx, func() error {
			if err := config.Bootstrap(); err != nil {
				return err
			}
			previous, err := os.ReadFile(config.LockSetupFile)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if string(previous) == lockSetupRules {
				return nil
			}
			changed = true
			return fileutil.AtomicWrite(config.LockSetupFile, []byte(lockSetupRules))
		}); err != nil {
			return Result{}, &Error{Code: "initialization_failed", Stage: "state", Message: err.Error(), Changed: changed}
		}
		if !service.BinaryInstalled(service.SingBox) {
			changed = true
			a.Progress("installing sing-box")
		}
		if err := core.Ensure(ctx, core.CompSingBox, ""); err != nil {
			return Result{}, &Error{Code: "initialization_failed", Stage: "prepare", Message: err.Error(), Changed: changed}
		}
		if _, err := os.Stat(config.SingBoxService); os.IsNotExist(err) {
			changed = true
			if err := service.ProvisionSingBox(ctx); err != nil {
				return Result{}, &Error{Code: "initialization_failed", Stage: "units", Message: err.Error(), Changed: true}
			}
		}
		if _, err := os.Stat(config.WatchdogService); os.IsNotExist(err) {
			changed = true
			if err := service.EnsureWatchdogRunningForCurrentBinary(ctx); err != nil {
				return Result{}, &Error{Code: "initialization_failed", Stage: "watchdog", Message: err.Error(), Changed: true}
			}
		}
		return Result{Changed: changed, Data: map[string]any{"initialized": true, "runtime": config.WorkDir}}, nil
	})
}

func ManagedServices(selector string, all bool) ([]service.Name, error) {
	if all && selector != "" || !all && selector == "" {
		return nil, Invalid("select one service or --all")
	}
	if all {
		return service.AllServices(), nil
	}
	for _, name := range service.AllServices() {
		if selector == string(name) {
			return []service.Name{name}, nil
		}
	}
	bindings, err := service.ShadowTLSServiceNames()
	if err != nil {
		return nil, err
	}
	for _, name := range bindings {
		if selector == name {
			return []service.Name{service.Name(name)}, nil
		}
	}
	return nil, Invalid("unknown managed service")
}

func (a *App) ServiceAction(ctx context.Context, action, selector string, all bool) (Result, error) {
	names, err := ManagedServices(selector, all)
	if err != nil {
		return Result{}, err
	}
	return a.Operation(ctx, func() (Result, error) {
		if _, err := a.Snapshot(ctx); err != nil {
			return Result{}, err
		}
		states, err := service.Snapshot(ctx, names...)
		if err != nil {
			return Result{}, err
		}
		if all && action == "stop" {
			for i, st := range states {
				if st.Name == service.Watchdog {
					states[0], states[i] = states[i], states[0]
					break
				}
			}
		}
		changed := false
		for _, st := range states {
			if !st.Installed {
				if all {
					continue
				}
				return Result{}, &Error{Code: "not_found", Message: "managed service is not installed"}
			}
			stopped := service.IsStopped(st.Name)
			if action == "stop" {
				if err := a.State(ctx, func() error { return service.SetStopped(st.Name, true) }); err != nil {
					return Result{}, err
				}
				changed = changed || !stopped || st.Running
				err = service.Stop(ctx, st.Name)
			} else {
				changed = changed || stopped || !st.Running || action == "restart"
				if action == "start" {
					err = service.Start(ctx, st.Name)
				} else {
					err = service.Restart(ctx, st.Name)
				}
				if err == nil {
					err = service.WaitReady(ctx, st.Name)
				}
				if err == nil {
					err = a.State(ctx, func() error { return service.SetStopped(st.Name, false) })
				}
			}
			if err != nil {
				return Result{}, &Error{Code: "service_action_failed", Stage: action, Message: err.Error(), Changed: changed, Data: map[string]any{"service": st.Name}}
			}
		}
		after, err := service.Snapshot(ctx, names...)
		if err != nil {
			return Result{}, &Error{Code: "service_observation_failed", Stage: "verify", Message: err.Error(), Changed: changed}
		}
		for _, st := range after {
			if !st.Installed && all {
				continue
			}
			if st.Running != (action != "stop") {
				return Result{}, &Error{Code: "service_action_failed", Stage: "verify", Message: "service did not reach requested state", Changed: changed, Data: after}
			}
		}
		return Result{Changed: changed, Data: after}, nil
	})
}

func (a *App) CertificateStatus(ctx context.Context) (Result, error) {
	if _, err := a.Snapshot(ctx); err != nil {
		return Result{}, err
	}
	return Result{Data: cert.Inspect()}, nil
}
func (a *App) CertificateEnsure(ctx context.Context, domain, email string) (Result, error) {
	if !cert.IsValidDomain(domain) {
		return Result{}, Invalid("invalid certificate domain")
	}
	if !cert.IsValidEmail(email) {
		return Result{}, Invalid("invalid certificate email")
	}
	return a.Operation(ctx, func() (Result, error) {
		if _, err := a.Snapshot(ctx); err != nil {
			return Result{}, err
		}
		if cert.CertExists(domain) {
			return Result{Data: map[string]any{"domain": domain, "ready": true}}, nil
		}
		if err := cert.EnsureCertificateState(ctx, domain, email, a.Progress, func(fn func() error) error { return a.State(ctx, fn) }); err != nil {
			return Result{}, &Error{Code: "certificate_failed", Stage: "certificate", Message: err.Error(), Changed: true}
		}
		return Result{Changed: true, Data: cert.Inspect()}, nil
	})
}
func Components(selector string, all bool) ([]core.Component, error) {
	if selector == "" {
		if all {
			return core.AllComponents(), nil
		}
		return nil, Invalid("select a core component or --all")
	}
	if all {
		return nil, Invalid("component and --all are mutually exclusive")
	}
	for _, c := range core.AllComponents() {
		if selector == string(c) {
			return []core.Component{c}, nil
		}
	}
	return nil, Invalid("unknown core component")
}
func (a *App) CoreVersions(ctx context.Context) (Result, error) {
	infos := make([]core.VersionInfo, 0, 4)
	for _, c := range core.AllComponents() {
		infos = append(infos, core.DetectVersion(ctx, core.BinaryPath(c), c))
	}
	return Result{Data: infos}, nil
}
func (a *App) CoreCheck(ctx context.Context, selector string) (Result, error) {
	components, err := Components(selector, selector == "")
	if err != nil {
		return Result{}, err
	}
	checks := make([]*core.UpdateCheck, 0, len(components))
	for _, c := range components {
		check, err := core.CheckUpdate(ctx, c, core.BinaryPath(c))
		if err != nil {
			return Result{}, err
		}
		checks = append(checks, check)
	}
	return Result{Data: checks}, nil
}
func (a *App) CoreUpdate(ctx context.Context, selector, version string, all bool) (Result, error) {
	components, err := Components(selector, all)
	if err != nil {
		return Result{}, err
	}
	if all && version != "" {
		return Result{}, Invalid("--all and --version are mutually exclusive")
	}
	return a.Operation(ctx, func() (Result, error) {
		if _, err := a.Snapshot(ctx); err != nil {
			return Result{}, err
		}
		bindings, err := service.ShadowTLSServiceNames()
		if err != nil {
			return Result{}, err
		}
		serviceNames := []service.Name{service.SingBox, service.Snell, service.CaddySub}
		for _, binding := range bindings {
			serviceNames = append(serviceNames, service.Name(binding))
		}
		states, err := service.Snapshot(ctx, serviceNames...)
		if err != nil {
			return Result{}, err
		}
		running := make(map[service.Name]bool)
		for _, st := range states {
			running[st.Name] = st.Running
		}
		changed := false
		applied := []core.Component{}
		for _, c := range components {
			if all {
				if _, err := os.Stat(core.BinaryPath(c)); os.IsNotExist(err) {
					continue
				}
			}
			a.Progress("checking " + string(c))
			check, err := core.ResolveUpdate(ctx, c, core.BinaryPath(c), version)
			if err == nil && check.UpdateAvail {
				a.Progress("updating " + string(c))
				err = core.ApplyUpdate(ctx, check)
				if err == nil {
					changed = true
					applied = append(applied, c)
				}
			}
			if err != nil {
				return Result{}, &Error{Code: "core_update_failed", Stage: "update", Message: err.Error(), Changed: changed, Data: map[string]any{"applied": applied, "pending": c}}
			}
			name := map[core.Component]service.Name{core.CompSingBox: service.SingBox, core.CompSnell: service.Snell, core.CompShadowTLS: service.ShadowTLS, core.CompCaddy: service.CaddySub}[c]
			restart := []service.Name{name}
			if c == core.CompShadowTLS {
				restart = nil
				for _, binding := range bindings {
					restart = append(restart, service.Name(binding))
				}
			}
			for _, target := range restart {
				if check.UpdateAvail && running[target] && !service.IsStopped(target) {
					if err := service.Restart(ctx, target); err != nil {
						return Result{}, &Error{Code: "service_restart_failed", Stage: "activate", Message: err.Error(), Changed: true, Data: map[string]any{"applied": applied, "pending": target}}
					}
					if err := service.WaitReady(ctx, target); err != nil {
						return Result{}, &Error{Code: "service_restart_failed", Stage: "verify", Message: err.Error(), Changed: true, Data: map[string]any{"applied": applied, "pending": target}}
					}
				}
			}
		}
		return Result{Changed: changed, Data: map[string]any{"updated": applied}}, nil
	})
}
func (a *App) SelfUpdate(ctx context.Context, current, version string, checkOnly bool) (Result, error) {
	if checkOnly {
		returnCheck, err := update.ResolveSelfUpdate(ctx, current, version)
		return Result{Data: returnCheck}, err
	}
	result, err := a.Operation(ctx, func() (Result, error) {
		a.Progress("checking go-proxy update")
		check, err := update.ResolveSelfUpdate(ctx, current, version)
		if err != nil {
			return Result{}, err
		}
		if !check.UpdateAvail {
			return Result{Data: check}, nil
		}
		a.Progress("updating go-proxy")
		if err := update.SelfUpdate(ctx, check); err != nil {
			return Result{}, err
		}
		return Result{Changed: true, Data: check}, nil
	})
	if err != nil || !result.Changed {
		return result, err
	}
	// The successor must not inherit a locked operation while starting.
	if service.IsInstalled(ctx, service.Watchdog) && !service.IsStopped(service.Watchdog) {
		if err := service.EnsureWatchdogRunningForCurrentBinary(ctx); err != nil {
			return Result{}, &Error{Code: "watchdog_restart_failed", Stage: "handoff", Message: err.Error(), Changed: true}
		}
	}
	return result, nil
}
func (a *App) Watchdog(ctx context.Context) (Result, error) {
	cfg := service.DefaultWatchdogConfig()
	cfg.Report = a.Progress
	cfg.Recover = func(ctx context.Context, name service.Name) error {
		_, err := a.Operation(ctx, func() (Result, error) {
			if service.IsStopped(name) {
				return Result{}, nil
			}
			if _, err := a.Snapshot(ctx); err != nil {
				return Result{}, err
			}
			return Result{Changed: true}, service.Restart(ctx, name)
		})
		return err
	}
	return Result{Silent: true}, service.RunWatchdog(ctx, cfg)
}
func (a *App) Log(ctx context.Context, selector string, lines, maxBytes int, follow bool, out io.Writer) (Result, error) {
	if selector == "" {
		selector = string(service.SingBox)
	}
	if _, err := ManagedServices(selector, false); err != nil {
		return Result{}, err
	}
	if lines <= 0 || maxBytes <= 0 {
		return Result{}, Invalid("log limits must be positive")
	}
	path, unit := logs.ServiceLogSource(selector)
	if follow {
		return Result{Silent: true}, logs.Follow(ctx, path, unit, lines, out)
	}
	content, source, err := logs.Read(ctx, path, unit, lines, maxBytes)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: map[string]any{"service": selector, "source": source, "content": content}}, nil
}
func (a *App) Uninstall(ctx context.Context, preview bool) (Result, error) {
	inventory := func() (map[string]any, error) {
		paths, err := service.OwnedUnitPaths()
		if err != nil {
			return nil, err
		}
		paths = append(paths, config.WorkDir, network.BBRSysctlPath, network.Fail2BanJailPath, config.LockSetupFile)
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return nil, err
		}
		return map[string]any{"paths": append(paths, executable), "firewall_table": "inet proxy_firewall"}, nil
	}
	if preview {
		data, err := inventory()
		return Result{Data: data}, err
	}
	return a.Operation(ctx, func() (Result, error) {
		data, err := inventory()
		if err != nil {
			return Result{}, err
		}
		a.Progress("removing go-proxy resources")
		fail := func(err error) (Result, error) {
			return Result{}, &Error{Code: "uninstall_failed", Stage: "remove", Message: err.Error(), Changed: true, Data: data}
		}
		if service.IsInstalled(ctx, service.Watchdog) {
			if err := service.Stop(ctx, service.Watchdog); err != nil {
				return fail(err)
			}
		}
		if err := network.RemoveFirewallRules(ctx); err != nil {
			return fail(err)
		}
		if _, err := os.Stat(network.Fail2BanJailPath); err == nil {
			if err := network.Fail2BanDisable(ctx); err != nil {
				return fail(err)
			}
		}
		if err := os.Remove(network.BBRSysctlPath); err != nil && !os.IsNotExist(err) {
			return fail(err)
		}
		if err := service.Uninstall(ctx, func(fn func() error) error { return a.State(ctx, fn) }); err != nil {
			return fail(err)
		}
		executable, err := os.Executable()
		if err != nil {
			return fail(err)
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return fail(err)
		}
		if err := os.Remove(executable); err != nil {
			return fail(err)
		}
		return Result{Changed: true, Data: data}, nil
	})
}
