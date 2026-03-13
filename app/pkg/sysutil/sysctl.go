package sysutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReadSysctl reads a kernel parameter value.
func ReadSysctl(ctx context.Context, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("empty sysctl key")
	}
	cmd := exec.CommandContext(ctx, "sysctl", "-n", key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("sysctl read %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// WriteSysctl sets a kernel parameter value.
func WriteSysctl(ctx context.Context, key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return fmt.Errorf("empty sysctl key")
	}
	cmd := exec.CommandContext(ctx, "sysctl", "-w", key+"="+value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sysctl write %s=%s: %s", key, value, strings.TrimSpace(string(out)))
	}
	return nil
}

// PersistSysctl writes a kernel parameter to /etc/sysctl.d/ for persistence across reboots.
func PersistSysctl(key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return fmt.Errorf("empty sysctl key")
	}

	dir := "/etc/sysctl.d"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sysctl.d: %w", err)
	}

	// Use a filename derived from the key.
	safeName := strings.NewReplacer(".", "-", "/", "-").Replace(key)
	path := filepath.Join(dir, "99-proxy-"+safeName+".conf")
	content := fmt.Sprintf("# Managed by proxy\n%s = %s\n", key, value)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
