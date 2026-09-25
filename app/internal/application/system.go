package application

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

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
		} else if !service.IsStopped(service.Watchdog) {
			// Re-running the installer replaces the binary and calls init; a
			// watchdog that kept running the replaced file would go on with
			// the old code until something restarted it. `gproxy update`
			// hands over the same way. An intentionally stopped watchdog is
			// left stopped.
			if stale, err := service.WatchdogRunsStaleBinary(ctx); err == nil && stale {
				changed = true
				a.Progress("restarting the watchdog on the new binary")
				if err := service.EnsureWatchdogRunningForCurrentBinary(ctx); err != nil {
					return Result{}, &Error{Code: "initialization_failed", Stage: "watchdog", Message: err.Error(), Changed: true}
				}
			}
		}
		return Result{Changed: changed, Data: map[string]any{"initialized": true, "runtime": config.WorkDir}}, nil
	})
}

// ManagedServiceNames lists the selectors a service action accepts, for the
// guidance an incomplete invocation prints. Dynamic ShadowTLS units are not
// included: they exist only while a binding does, so naming them here would
// promise selectors that may not resolve.
func ManagedServiceNames() []string {
	all := service.AllServices()
	names := make([]string, 0, len(all))
	for _, name := range all {
		names = append(names, string(name))
	}
	return names
}

// serviceAliases are the short names the dashboard prints for a service,
// accepted wherever a service is selected: a name read off `gproxy status`
// has to work in the next command.
var serviceAliases = map[string]service.Name{
	"snell":    service.Snell,
	"caddy":    service.CaddySub,
	"watchdog": service.Watchdog,
}

// canonicalService turns a short dashboard name into the unit name.
func canonicalService(selector string) string {
	if name, ok := serviceAliases[selector]; ok {
		return string(name)
	}
	return selector
}

func ManagedServices(selector string, all bool) ([]service.Name, error) {
	if all && selector != "" || !all && selector == "" {
		return nil, Invalid("select one service or --all")
	}
	selector = canonicalService(selector)
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

func (a *App) ServiceStatus(ctx context.Context) (Result, error) {
	if _, err := a.Snapshot(ctx); err != nil {
		return Result{}, err
	}
	observationCtx, cancel := ObservationContext(ctx)
	defer cancel()
	states, err := service.Snapshot(observationCtx)
	if err != nil {
		return Result{}, err
	}
	return Result{Data: map[string]any{"services": states}}, nil
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

// CertificateDomain is the domain the host's certificate is for, empty when
// none is configured.
func (a *App) CertificateDomain() string { return cert.ReadDomain() }

// CertificateEnsure makes sure a certificate exists for domain, or for the
// configured domain when none is named. An existing certificate is reported
// as it stands, with its expiry; renewal is Caddy's.
func (a *App) CertificateEnsure(ctx context.Context, domain, email string) (Result, error) {
	if domain == "" {
		domain = cert.ReadDomain()
	}
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
			status := cert.Inspect()
			if status.Domain != domain {
				status = cert.Status{Domain: domain, Ready: true}
			}
			return Result{Data: status}, nil
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
		infos = append(infos, core.InstalledVersion(ctx, core.BinaryPath(c), c))
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
		check.Updated = true
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
	selector = canonicalService(selector)
	if _, err := ManagedServices(selector, false); err != nil {
		return Result{}, err
	}
	// shadow-tls names a group: one unit per wrapped node, each with its own
	// log. The group itself has none, so its log is the one unit's, and a
	// choice when there are several.
	if selector == string(service.ShadowTLS) {
		units, err := service.ShadowTLSServiceNames()
		if err != nil {
			return Result{}, err
		}
		switch len(units) {
		case 0:
			return Result{}, Invalid("no shadow-tls service is installed")
		case 1:
			selector = units[0]
		default:
			return Result{}, Invalid("several shadow-tls services; select one: " + strings.Join(units, ", "))
		}
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

// uninstallScope is every path uninstall owns, whether or not it exists on
// this host: the units, the runtime, the lock directory, the files placed
// elsewhere and the executable.
func (a *App) uninstallScope() ([]string, error) {
	paths, err := service.OwnedUnitPaths()
	if err != nil {
		return nil, err
	}
	paths = append(paths, config.WorkDir, a.LockDir, network.BBRSysctlPath, network.Fail2BanJailPath, config.LockSetupFile,
		config.BashCompletionPath, config.ZshCompletionPath)
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	return append(paths, executable), nil
}

func (a *App) Uninstall(ctx context.Context, preview bool) (Result, error) {
	inventory := func() (map[string]any, error) {
		paths, err := a.uninstallScope()
		if err != nil {
			return nil, err
		}
		// Only what is there: a preview listing paths that do not exist says
		// uninstall would remove things it will not find.
		existing := []string{}
		for _, path := range paths {
			if _, err := os.Lstat(path); err == nil {
				existing = append(existing, path)
			}
		}
		data := map[string]any{"paths": existing}
		if managed, err := network.FirewallManaged(ctx); err == nil && managed {
			data["firewall_table"] = "inet proxy_firewall"
		}
		if hasMarkedBlock(config.SystemBashrc, config.BashrcCompletionBeginMark) {
			data["bashrc_block"] = config.SystemBashrc
		}
		return data, nil
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
			if err := network.Fail2BanRemoveJail(ctx); err != nil {
				return fail(err)
			}
		}
		if err := os.Remove(network.BBRSysctlPath); err != nil && !os.IsNotExist(err) {
			return fail(err)
		}
		// Completion scripts are absent when the CLI was built from source
		// rather than installed, so a missing file is not a failure. Only these
		// two exact paths are removed; the directories belong to the shells.
		for _, path := range []string{config.BashCompletionPath, config.ZshCompletionPath} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fail(err)
			}
		}
		if err := removeMarkedBlock(config.SystemBashrc, config.BashrcCompletionBeginMark, config.BashrcCompletionEndMark); err != nil {
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
		// Last, because everything above ran under these locks. Unlinking a
		// flocked file leaves this process's lock valid and the descriptors
		// closable, while a concurrent gproxy fails to open them at all --
		// which is the right answer once the runtime is gone. Left behind, the
		// directory outlived every reinstall on a host that had not rebooted.
		if err := os.RemoveAll(a.LockDir); err != nil {
			return fail(err)
		}
		return Result{Changed: true, Data: data}, nil
	})
}

// removeMarkedBlock deletes the lines from begin to end, both included, from a
// file this program does not own, leaving every other line as it was. A file
// without the block, or no file at all, is not an error.
// hasMarkedBlock reports whether path carries the line that opens a block
// uninstall removes.
func hasMarkedBlock(path, begin string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimRight(line, "\r") == begin {
			return true
		}
	}
	return false
}

func removeMarkedBlock(path, begin, end string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(data), "\n")
	kept := make([]string, 0, len(lines))
	inside, found := false, false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\n")
		switch {
		case !inside && trimmed == begin:
			inside, found = true, true
		case inside && trimmed == end:
			inside = false
		case !inside:
			kept = append(kept, line)
		}
	}
	if !found || inside {
		return nil
	}
	return fileutil.AtomicWriteMode(path, []byte(strings.Join(kept, "")), info.Mode().Perm())
}
