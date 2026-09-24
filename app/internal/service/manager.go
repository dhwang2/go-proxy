package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go-proxy/internal/config"
	"go-proxy/pkg/textutil"
)

// Name represents a managed service.
type Name string

// An explicit stop is remembered so automatic recovery does not undo it, which
// makes it this program's own state rather than anything a core reads.
var stoppedDir = config.DataDir

const (
	SingBox   Name = "sing-box"
	Snell     Name = "snell-v6"
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
	Name      Name   `json:"name"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
	Installed bool   `json:"installed"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

// Start starts a systemd service.
func Start(ctx context.Context, name Name) error {
	if name == ShadowTLS {
		return systemctlShadowTLSGroup(ctx, "start")
	}
	return systemctl(ctx, "start", string(name))
}

// Stop stops a systemd service.
func Stop(ctx context.Context, name Name) error {
	if name == ShadowTLS {
		return systemctlShadowTLSGroup(ctx, "stop")
	}
	return systemctl(ctx, "stop", string(name))
}

// Restart restarts a systemd service.
func Restart(ctx context.Context, name Name) error {
	if name == ShadowTLS {
		return systemctlShadowTLSGroup(ctx, "restart")
	}
	return systemctl(ctx, "restart", string(name))
}

// Enable enables a systemd service.
func Enable(ctx context.Context, name Name) error {
	if name == ShadowTLS {
		return systemctlShadowTLSGroup(ctx, "enable")
	}
	return systemctl(ctx, "enable", string(name))
}

// Disable disables a systemd service.
func Disable(ctx context.Context, name Name) error {
	if name == ShadowTLS {
		return systemctlShadowTLSGroup(ctx, "disable")
	}
	return systemctl(ctx, "disable", string(name))
}

// IsInstalled checks whether a systemd unit file exists for the service.
func IsInstalled(ctx context.Context, name Name) bool {
	if name == ShadowTLS {
		names, err := ShadowTLSServiceNames()
		return err == nil && len(names) > 0
	}
	_, err := systemctlOutput(ctx, "cat", string(name))
	return err == nil
}

// BinaryInstalled checks whether the managed binary for a service exists on disk.
func BinaryInstalled(name Name) bool {
	var path string
	switch name {
	case SingBox:
		path = config.SingBoxBin
	case Snell:
		path = config.SnellBin
	case ShadowTLS:
		path = config.ShadowTLSBin
	case CaddySub:
		path = config.CaddyBin
	default:
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

// Snapshot observes all selected units with one systemctl call.
func Snapshot(ctx context.Context, names ...Name) ([]Status, error) {
	if len(names) == 0 {
		names = AllServices()
	}
	groups := make(map[Name][]string, len(names))
	var units []string
	for _, name := range names {
		if name == ShadowTLS {
			children, err := ShadowTLSServiceNames()
			if err != nil {
				return nil, err
			}
			groups[name] = children
			units = append(units, children...)
		} else {
			groups[name] = []string{string(name)}
			units = append(units, string(name))
		}
	}
	result := make([]Status, 0, len(names))
	if len(units) == 0 {
		for _, name := range names {
			result = append(result, Status{Name: name, State: "missing"})
		}
		return result, nil
	}
	args := []string{"show", "--no-pager", "--property=Id,LoadState,ActiveState,UnitFileState"}
	args = append(args, units...)
	output, commandErr := systemctlOutput(ctx, args...)
	parsed := make(map[string]Status)
	for _, block := range strings.Split(strings.TrimSpace(output), "\n\n") {
		fields := make(map[string]string)
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				fields[key] = value
			}
		}
		id := strings.TrimSuffix(fields["Id"], ".service")
		if id == "" || fields["LoadState"] == "" {
			continue
		}
		st := Status{Name: Name(id), Installed: fields["LoadState"] != "not-found", Running: fields["ActiveState"] == "active", Enabled: fields["UnitFileState"] == "enabled", State: fields["ActiveState"]}
		if !st.Installed {
			st.State = "missing"
		}
		if fields["LoadState"] != "loaded" && fields["LoadState"] != "not-found" {
			st.Error = "unit could not be loaded"
		}
		parsed[id] = st
	}
	for _, name := range names {
		members := groups[name]
		st := Status{Name: name, State: "missing"}
		if len(members) > 0 {
			st.Installed = true
			st.Running = true
			st.Enabled = true
			st.State = "active"
			for _, unit := range members {
				child, ok := parsed[unit]
				if !ok {
					st.Running = false
					st.Enabled = false
					st.Installed = false
					st.State = "unknown"
					st.Error = "service observation failed"
					continue
				}
				st.Installed = st.Installed && child.Installed
				st.Running = st.Running && child.Running
				st.Enabled = st.Enabled && child.Enabled
				if child.State != "active" {
					st.State = child.State
				}
				if child.Error != "" {
					st.Error = child.Error
				}
			}
		}
		result = append(result, st)
	}
	if commandErr != nil && len(parsed) == 0 {
		return result, fmt.Errorf("service observation failed: %w", commandErr)
	}
	for _, st := range result {
		if st.Error != "" {
			return result, fmt.Errorf("service observation incomplete")
		}
	}
	return result, nil
}

func GetStatus(ctx context.Context, name Name) (*Status, error) {
	states, err := Snapshot(ctx, name)
	if err != nil {
		return nil, err
	}
	if !states[0].Installed {
		return nil, nil
	}
	return &states[0], nil
}

func WaitReady(ctx context.Context, name Name) error {
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	observations := 0
	for {
		states, err := Snapshot(waitCtx, name)
		if err != nil {
			return fmt.Errorf("observe %s readiness: %w", name, err)
		}
		st := states[0]
		if !st.Installed || st.State == "failed" {
			return fmt.Errorf("service %s did not become active", name)
		}
		if st.Running {
			observations++
		} else {
			observations = 0
		}
		if observations >= 2 {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for %s readiness: %w", name, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func SetStopped(name Name, stopped bool) error {
	if name == ShadowTLS {
		names, err := ShadowTLSServiceNames()
		if err != nil {
			return err
		}
		for _, unit := range names {
			if err := setStoppedFile(Name(unit), stopped); err != nil {
				return err
			}
		}
	}
	if !stopped && strings.HasPrefix(string(name), "shadow-tls-") {
		if err := setStoppedFile(ShadowTLS, false); err != nil {
			return err
		}
	}
	return setStoppedFile(name, stopped)
}
func setStoppedFile(name Name, stopped bool) error {
	path := filepath.Join(stoppedDir, ".stopped-"+string(name))
	if stopped {
		return os.WriteFile(path, []byte{}, 0600)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func IsStopped(name Name) bool {
	if strings.HasPrefix(string(name), "shadow-tls-") {
		if _, err := os.Stat(filepath.Join(stoppedDir, ".stopped-"+string(ShadowTLS))); err == nil {
			return true
		}
	}
	_, err := os.Stat(filepath.Join(stoppedDir, ".stopped-"+string(name)))
	return err == nil
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
		// Stripped where it enters: this text is quoted into an error message
		// that reaches a JSON envelope, which carries no escape sequences.
		msg := strings.TrimSpace(textutil.CleanText(string(out)))
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

func systemctlShadowTLSGroup(ctx context.Context, action string) error {
	names, err := ShadowTLSServiceNames()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("service %s is not installed", ShadowTLS)
	}
	var errs []string
	for _, name := range names {
		if err := systemctl(ctx, action, name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
