package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-proxy/internal/config"
)

func OwnedUnitPaths() ([]string, error) {
	paths := []string{config.WatchdogService, config.SingBoxService, config.SnellService, config.CaddySubService}
	bindings, err := shadowTLSUnitPaths()
	if err != nil {
		return nil, err
	}
	paths = append(paths, bindings...)
	return ownedUnitPaths(paths, config.BinDir, config.WatchdogService)
}

func ownedUnitPaths(paths []string, binDir, watchdogPath string) ([]string, error) {
	var existing []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(data), binDir+"/") && path != watchdogPath {
			return nil, fmt.Errorf("unit is not owned by go-proxy: %s", path)
		}
		if path == watchdogPath && !strings.Contains(string(data), " watchdog") {
			return nil, fmt.Errorf("watchdog unit is not owned by go-proxy")
		}
		existing = append(existing, path)
	}
	return existing, nil
}

func Uninstall(ctx context.Context, state func(func() error) error) error {
	paths, err := OwnedUnitPaths()
	if err != nil {
		return err
	}
	return uninstallOwned(ctx, paths, config.WorkDir, config.LockSetupFile, state)
}

func uninstallOwned(ctx context.Context, paths []string, runtimeDir, lockSetupFile string, state func(func() error) error) error {
	// The watchdog is first so it cannot recover units during removal.
	for _, path := range paths {
		unit := strings.TrimSuffix(filepath.Base(path), ".service")
		if err := systemctl(ctx, "stop", unit); err != nil {
			return err
		}
		if err := systemctl(ctx, "disable", unit); err != nil {
			return err
		}
	}
	if err := state(func() error {
		for _, path := range paths {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err := os.Remove(lockSetupFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.RemoveAll(runtimeDir)
	}); err != nil {
		return err
	}
	if len(paths) > 0 {
		if err := DaemonReload(ctx); err != nil {
			return err
		}
		// systemd keeps the failure record of a unit whose file is gone, so an
		// uninstalled watchdog stayed visible in `systemctl --failed` as
		// not-found. Reset by name, never globally: another program's failed
		// unit is not this one's to clear. Best effort, because a unit that
		// never failed is not loaded and systemctl says so.
		units := make([]string, 0, len(paths))
		for _, path := range paths {
			units = append(units, filepath.Base(path))
		}
		_ = systemctl(ctx, append([]string{"reset-failed"}, units...)...)
	}
	return nil
}
