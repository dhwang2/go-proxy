package service

import (
	"context"
	"fmt"
	"os"
	"time"
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

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		st, err := GetStatus(waitCtx, Watchdog)
		if err == nil && st != nil && st.Running {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("watchdog did not become active")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func EnsureWatchdogRunningForCurrentBinary(ctx context.Context) error {
	path, err := os.Executable()
	if err != nil || path == "" {
		path = "/usr/bin/gproxy"
	}
	return EnsureWatchdogRunning(ctx, path)
}
