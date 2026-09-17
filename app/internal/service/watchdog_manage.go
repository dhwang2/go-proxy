package service

import (
	"context"
	"os"
)

func EnsureWatchdogRunning(ctx context.Context, proxyBin string) error {
	if proxyBin == "" {
		proxyBin = "/usr/bin/gproxy"
	}
	if err := ProvisionWatchdog(ctx, proxyBin); err != nil {
		return err
	}
	if err := Enable(ctx, Watchdog); err != nil {
		return err
	}
	if err := Restart(ctx, Watchdog); err != nil {
		return err
	}

	return WaitReady(ctx, Watchdog)
}

func EnsureWatchdogRunningForCurrentBinary(ctx context.Context) error {
	path, err := os.Executable()
	if err != nil || path == "" {
		path = "/usr/bin/gproxy"
	}
	return EnsureWatchdogRunning(ctx, path)
}
