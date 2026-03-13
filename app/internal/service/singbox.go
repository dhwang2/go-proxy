package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ValidateConfig runs sing-box check against the given config path.
func ValidateConfig(ctx context.Context, configPath string) error {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return fmt.Errorf("empty config path")
	}
	cmd := exec.CommandContext(ctx, "sing-box", "check", "-c", configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("sing-box check failed: %s", msg)
	}
	return nil
}
