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

	// Stop legacy static shadow-tls.service that IsInstalled may miss.
	_ = systemctl(ctx, "stop", "shadow-tls")
	_ = systemctl(ctx, "disable", "shadow-tls")

	// Remove nftables tables.
	removeNftTables()

	// Revert sysctl changes (BBR settings in /etc/sysctl.conf).
	revertSysctl()

	// Remove unit files.
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

	// Flush journald entries.
	clearJournalEntries()

	return errors.Join(errs...)
}

// removeNftTables removes all nftables tables created by this application.
func removeNftTables() {
	nft, err := exec.LookPath("nft")
	if err != nil {
		return
	}
	exec.Command(nft, "delete", "table", "inet", "proxy_firewall").Run()
	exec.Command(nft, "delete", "table", "inet", "proxy-filter").Run()
}

// revertSysctl removes gproxy-managed entries from /etc/sysctl.conf.
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

	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}

	result := strings.Join(kept, "\n") + "\n"
	_ = os.WriteFile(path, []byte(result), 0644)
	_ = exec.Command("sysctl", "-p").Run()
}

// clearJournalEntries rotates and vacuums the journal to remove old entries.
func clearJournalEntries() {
	_ = exec.Command("journalctl", "--rotate").Run()
	_ = exec.Command("journalctl", "--vacuum-time=1s").Run()
}
