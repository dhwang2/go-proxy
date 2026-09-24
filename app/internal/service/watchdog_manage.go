package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// procRoot is where process executables are read; tests point it elsewhere.
var procRoot = "/proc"

// WatchdogRunsStaleBinary reports whether the running watchdog executes a file
// other than this binary: one replaced on disk since it started, which the
// kernel reports with a " (deleted)" suffix, or another path. A watchdog that is
// not running is not stale; starting it is someone else's decision.
func WatchdogRunsStaleBinary(ctx context.Context) (bool, error) {
	out, err := systemctlOutput(ctx, "show", "--property=MainPID", "--value", string(Watchdog))
	if err != nil {
		return false, err
	}
	pid := strings.TrimSpace(out)
	if pid == "" || pid == "0" {
		return false, nil
	}
	running, err := os.Readlink(filepath.Join(procRoot, pid, "exe"))
	if err != nil {
		return false, err
	}
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return running != self, nil
}
