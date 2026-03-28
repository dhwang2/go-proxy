package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"go-proxy/internal/config"
)

// Uninstall stops and removes all managed services and configuration.
func Uninstall(ctx context.Context) error {
	var errs []error

	// Collect all unit names for cleanup later.
	var unitNames []string
	for _, svc := range AllServices() {
		unitNames = append(unitNames, string(svc))
	}
	if names, err := ShadowTLSServiceNames(); err == nil {
		unitNames = append(unitNames, names...)
	}

	// Stop and disable all installed services.
	for _, svc := range AllServices() {
		if !IsInstalled(ctx, svc) {
			continue
		}
		if err := Stop(ctx, svc); err != nil {
			errs = append(errs, err)
		}
		if err := Disable(ctx, svc); err != nil {
			errs = append(errs, err)
		}
	}

	// Stop legacy static shadow-tls.service that IsInstalled may miss
	// (the glob only matches shadow-tls-*.service, not the static name).
	_ = systemctl(ctx, "stop", "shadow-tls")
	_ = systemctl(ctx, "disable", "shadow-tls")

	// Remove firewall rules (nftables table, iptables chains).
	removeFirewallRules()

	// Revert sysctl changes (BBR settings in /etc/sysctl.conf).
	revertSysctl()

	// Remove unit files (include ShadowTLSService which may exist as a static unit).
	unitFiles := []string{
		config.SingBoxService,
		config.SnellService,
		config.ShadowTLSService,
		config.CaddySubService,
		config.WatchdogService,
	}
	if shadowTLSUnits, err := shadowTLSUnitPaths(); err == nil {
		unitFiles = append(unitFiles, shadowTLSUnits...)
	}
	for _, f := range unitFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}

	if err := DaemonReload(ctx); err != nil {
		errs = append(errs, err)
	}

	// Reset failed state so systemd forgets about removed units.
	for _, unit := range unitNames {
		_ = exec.CommandContext(ctx, "systemctl", "reset-failed", unit).Run()
	}

	// Remove work directory (configs, logs, binaries, certs, cache).
	if err := os.RemoveAll(config.WorkDir); err != nil {
		errs = append(errs, err)
	}

	// Flush journald entries for managed units.
	clearJournalEntries()

	return errors.Join(errs...)
}

// removeFirewallRules removes all firewall rules created by this application.
func removeFirewallRules() {
	// Remove nftables tables.
	if _, err := exec.LookPath("nft"); err == nil {
		exec.Command("nft", "delete", "table", "inet", "proxy_firewall").Run()
		exec.Command("nft", "delete", "table", "inet", "proxy-filter").Run()
	}

	// Remove iptables chains.
	for _, spec := range []struct {
		cmd   string
		chain string
	}{
		{"iptables", "PROXY_FIREWALL_INPUT"},
		{"ip6tables", "PROXY_FIREWALL_INPUT6"},
	} {
		if _, err := exec.LookPath(spec.cmd); err != nil {
			continue
		}
		// Detach from INPUT chain.
		for exec.Command(spec.cmd, "-C", "INPUT", "-j", spec.chain).Run() == nil {
			exec.Command(spec.cmd, "-D", "INPUT", "-j", spec.chain).Run()
		}
		// Flush and delete the chain.
		exec.Command(spec.cmd, "-F", spec.chain).Run()
		exec.Command(spec.cmd, "-X", spec.chain).Run()
	}
}

// revertSysctl removes gproxy-managed entries from /etc/sysctl.conf.
// Only lines tagged with "# gproxy-managed" are removed; user-written entries
// for the same keys are preserved.
func revertSysctl() {
	const path = "/etc/sysctl.conf"
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var kept []string
	changed := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "# gproxy-managed") {
			changed = true
			continue
		}
		kept = append(kept, line)
	}

	if !changed {
		return
	}

	// Remove trailing empty lines.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}

	result := strings.Join(kept, "\n") + "\n"
	_ = os.WriteFile(path, []byte(result), 0644)

	// Reload sysctl so kernel picks up the reverted values.
	_ = exec.Command("sysctl", "-p").Run()
}

// clearJournalEntries rotates and vacuums the journal to remove old entries.
func clearJournalEntries() {
	_ = exec.Command("journalctl", "--rotate").Run()
	_ = exec.Command("journalctl", "--vacuum-time=1s").Run()
}
