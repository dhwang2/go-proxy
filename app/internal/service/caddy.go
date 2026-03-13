package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CaddyReload sends SIGUSR1 to caddy for a graceful reload via systemctl.
func CaddyReload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "reload", "caddy-sub.service")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("caddy reload: %s", msg)
	}
	return nil
}
