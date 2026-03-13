package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dhwang2/go-proxy/pkg/sysutil"
)

// Uninstall stops all services, disables them, and removes config/binary files.
func Uninstall(ctx context.Context, workDir string) error {
	if workDir == "" {
		workDir = "/etc/go-proxy"
	}

	mgr := NewManager(Options{WorkDir: workDir})
	specs := mgr.serviceSpecs()

	var warnings []string

	// Stop all services.
	for _, spec := range specs {
		if err := sysutil.ServiceAction(ctx, "stop", spec.Unit); err != nil {
			warnings = append(warnings, fmt.Sprintf("stop %s: %v", spec.Unit, err))
		}
	}

	// Verify services stopped.
	for _, spec := range specs {
		if state, err := sysutil.ServiceState(ctx, spec.Unit); err == nil && state == "active" {
			warnings = append(warnings, fmt.Sprintf("%s still active after stop", spec.Unit))
		}
	}

	// Disable all services.
	for _, spec := range specs {
		if err := sysutil.ServiceAction(ctx, "disable", spec.Unit); err != nil {
			warnings = append(warnings, fmt.Sprintf("disable %s: %v", spec.Unit, err))
		}
	}

	// Remove systemd unit files for shadow-tls instances.
	units, _ := filepath.Glob("/etc/systemd/system/shadow-tls*.service")
	for _, u := range units {
		if err := os.Remove(u); err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("remove %s: %v", u, err))
		}
	}

	// Reload systemd daemon after removing units.
	if err := exec.CommandContext(ctx, "systemctl", "daemon-reload").Run(); err != nil {
		warnings = append(warnings, fmt.Sprintf("daemon-reload: %v", err))
	}

	// Remove the proxy config directory.
	if err := os.RemoveAll(workDir); err != nil {
		return fmt.Errorf("remove %s: %w (warnings: %v)", workDir, err, warnings)
	}

	// Remove the proxy binary.
	proxyBin := "/usr/bin/proxy"
	if err := os.Remove(proxyBin); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w (warnings: %v)", proxyBin, err, warnings)
	}

	return nil
}
