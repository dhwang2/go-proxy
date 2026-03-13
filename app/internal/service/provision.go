package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// serviceUnit describes a systemd service file to provision.
type serviceUnit struct {
	Name     string // e.g. "sing-box"
	Unit     string // e.g. "sing-box.service"
	Content  string // systemd unit file content
	BinCheck string // binary path that must exist before provisioning
}

// EnsureServiceFiles creates missing systemd service files for all core
// services whose binaries are present, then reloads the systemd daemon.
// It returns the list of units that were newly created.
func (m *Manager) EnsureServiceFiles(ctx context.Context) ([]string, error) {
	workDir := m.options.WorkDir
	if workDir == "" {
		workDir = "/etc/go-proxy"
	}
	binDir := filepath.Join(workDir, "bin")
	logDir := filepath.Join(workDir, "logs")
	confDir := filepath.Join(workDir, "conf")

	// Ensure directories exist.
	for _, dir := range []string{binDir, logDir, confDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	confFile := filepath.Join(confDir, "config.json")
	units := []serviceUnit{
		{
			Name:     "sing-box",
			Unit:     "sing-box.service",
			BinCheck: filepath.Join(binDir, "sing-box"),
			Content: fmt.Sprintf(`[Unit]
Description=sing-box service
After=network.target nss-lookup.target

[Service]
ExecStart=%s run -c %s
Restart=on-failure
LimitNOFILE=infinity
StandardOutput=append:%s/sing-box.service.log
StandardError=append:%s/sing-box.service.log

[Install]
WantedBy=multi-user.target
`, filepath.Join(binDir, "sing-box"), confFile, logDir, logDir),
		},
		{
			Name:     "snell-v5",
			Unit:     "snell-v5.service",
			BinCheck: filepath.Join(binDir, "snell-server"),
			Content: fmt.Sprintf(`[Unit]
Description=snell v5 service
After=network.target

[Service]
ExecStart=%s -c %s/snell-v5.conf
Restart=on-failure
StandardOutput=append:%s/snell-v5.service.log
StandardError=append:%s/snell-v5.service.log

[Install]
WantedBy=multi-user.target
`, filepath.Join(binDir, "snell-server"), workDir, logDir, logDir),
		},
	}

	var created []string
	daemonNeedsReload := false

	for _, u := range units {
		unitPath := filepath.Join("/etc/systemd/system", u.Unit)
		if fileExists(unitPath) {
			continue
		}
		if u.BinCheck != "" && !fileExists(u.BinCheck) {
			continue
		}
		if err := os.WriteFile(unitPath, []byte(u.Content), 0o644); err != nil {
			return created, fmt.Errorf("write %s: %w", unitPath, err)
		}
		created = append(created, u.Unit)
		daemonNeedsReload = true
	}

	if daemonNeedsReload {
		_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
		for _, unit := range created {
			_ = exec.CommandContext(ctx, "systemctl", "enable", unit).Run()
		}
	}

	return created, nil
}

// EnsureShadowTLSServiceFile creates a systemd service file for a single
// shadow-tls instance. Returns true if the file was newly created.
func (m *Manager) EnsureShadowTLSServiceFile(ctx context.Context, serviceName string, execStart string) (bool, error) {
	workDir := m.options.WorkDir
	if workDir == "" {
		workDir = "/etc/go-proxy"
	}
	logDir := filepath.Join(workDir, "logs")

	if strings.TrimSpace(serviceName) == "" || strings.TrimSpace(execStart) == "" {
		return false, fmt.Errorf("service name and exec start are required")
	}

	unitPath := filepath.Join("/etc/systemd/system", serviceName+".service")
	if fileExists(unitPath) {
		return false, nil
	}

	content := fmt.Sprintf(`[Unit]
Description=shadow-tls service (%s)
After=network.target

[Service]
ExecStart=%s
Restart=on-failure
LimitNOFILE=infinity
StandardOutput=append:%s/shadow-tls.service.log
StandardError=append:%s/shadow-tls.service.log

[Install]
WantedBy=multi-user.target
`, serviceName, execStart, logDir, logDir)

	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", unitPath, err)
	}
	_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	_ = exec.CommandContext(ctx, "systemctl", "enable", serviceName+".service").Run()
	return true, nil
}

// Bootstrap ensures the directory structure, downloads missing core binaries,
// provisions systemd service files, and enables/starts services.
func (m *Manager) Bootstrap(ctx context.Context) ([]OperationResult, error) {
	workDir := m.options.WorkDir
	if workDir == "" {
		workDir = "/etc/go-proxy"
	}
	binDir := filepath.Join(workDir, "bin")
	logDir := filepath.Join(workDir, "logs")
	confDir := filepath.Join(workDir, "conf")

	results := make([]OperationResult, 0, 8)

	// Ensure directory structure.
	for _, dir := range []string{binDir, logDir, confDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			results = append(results, OperationResult{Name: "dirs", Status: "failed", Message: err.Error()})
			return results, nil
		}
	}
	results = append(results, OperationResult{Name: "dirs", Status: "ok"})

	// Provision systemd service files.
	created, err := m.EnsureServiceFiles(ctx)
	if err != nil {
		results = append(results, OperationResult{Name: "systemd", Status: "failed", Message: err.Error()})
	} else if len(created) > 0 {
		results = append(results, OperationResult{Name: "systemd", Status: "ok", Message: "created: " + strings.Join(created, ", ")})
	} else {
		results = append(results, OperationResult{Name: "systemd", Status: "ok", Message: "all service files present"})
	}

	// Start services whose binaries exist.
	specs := m.serviceSpecs()
	for _, spec := range specs {
		if spec.Bin != "" && !fileExists(spec.Bin) {
			results = append(results, OperationResult{Name: spec.Name, Status: "skipped", Message: "binary not installed"})
			continue
		}
		unitPath := filepath.Join("/etc/systemd/system", spec.Unit)
		if !fileExists(unitPath) {
			results = append(results, OperationResult{Name: spec.Name, Status: "skipped", Message: "no service file"})
			continue
		}
		if err := exec.CommandContext(ctx, "systemctl", "start", spec.Unit).Run(); err != nil {
			results = append(results, OperationResult{Name: spec.Name, Status: "failed", Message: err.Error()})
		} else {
			results = append(results, OperationResult{Name: spec.Name, Status: "ok", Message: "started"})
		}
	}

	return results, nil
}
