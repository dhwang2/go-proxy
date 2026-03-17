package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Name represents a managed service.
type Name string

const (
	SingBox   Name = "sing-box"
	Snell     Name = "snell-v5"
	ShadowTLS Name = "shadow-tls"
	CaddySub  Name = "caddy-sub"
	Watchdog  Name = "proxy-watchdog"
)

// AllServices returns all managed service names.
func AllServices() []Name {
	return []Name{SingBox, Snell, ShadowTLS, CaddySub, Watchdog}
}

// Status holds the status of a systemd service.
type Status struct {
	Name      Name
	Running   bool
	Enabled   bool
	ExitCode  string
	MainPID   int
	StartedAt string
}

// Start starts a systemd service.
func Start(ctx context.Context, name Name) error {
	return systemctl(ctx, "start", string(name))
}

// Stop stops a systemd service.
func Stop(ctx context.Context, name Name) error {
	return systemctl(ctx, "stop", string(name))
}

// Restart restarts a systemd service.
func Restart(ctx context.Context, name Name) error {
	return systemctl(ctx, "restart", string(name))
}

// Enable enables a systemd service.
func Enable(ctx context.Context, name Name) error {
	return systemctl(ctx, "enable", string(name))
}

// Disable disables a systemd service.
func Disable(ctx context.Context, name Name) error {
	return systemctl(ctx, "disable", string(name))
}

// IsInstalled checks whether a systemd unit file exists for the service.
func IsInstalled(ctx context.Context, name Name) bool {
	_, err := systemctlOutput(ctx, "cat", string(name))
	return err == nil
}

// GetStatus returns the status of a systemd service.
// Returns nil if the service is not installed.
func GetStatus(ctx context.Context, name Name) (*Status, error) {
	if !IsInstalled(ctx, name) {
		return nil, nil
	}

	unit := string(name)
	st := &Status{Name: name}

	// Check if active.
	out, err := systemctlOutput(ctx, "is-active", unit)
	st.Running = err == nil && strings.TrimSpace(out) == "active"

	// Check if enabled.
	out, err = systemctlOutput(ctx, "is-enabled", unit)
	st.Enabled = err == nil && strings.TrimSpace(out) == "enabled"

	return st, nil
}

// DaemonReload runs systemctl daemon-reload.
func DaemonReload(ctx context.Context) error {
	return systemctl(ctx, "daemon-reload")
}

func systemctl(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "Interactive authentication required") {
			return fmt.Errorf("permission denied, try running with sudo")
		}
		if strings.Contains(msg, "not found") {
			return fmt.Errorf("service %s is not installed", args[len(args)-1])
		}
		return fmt.Errorf("systemctl %s: %s: %s", strings.Join(args, " "), err, msg)
	}
	return nil
}

func systemctlOutput(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
