package service

import (
	"context"
	"errors"
	"os"

	"go-proxy/internal/config"
)

// Uninstall stops and removes all managed services and configuration.
func Uninstall(ctx context.Context) error {
	var errs []error

	// Stop all installed services.
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

	// Remove unit files.
	unitFiles := []string{
		config.SingBoxService,
		config.SnellService,
		config.ShadowTLSService,
		config.CaddySubService,
		config.WatchdogService,
	}
	for _, f := range unitFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}

	if err := DaemonReload(ctx); err != nil {
		errs = append(errs, err)
	}

	// Remove work directory.
	if err := os.RemoveAll(config.WorkDir); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
