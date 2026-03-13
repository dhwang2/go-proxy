package sysutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func ServiceState(ctx context.Context, unit string) (string, error) {
	if strings.TrimSpace(unit) == "" {
		return "", errors.New("empty unit")
	}
	out, err := runCommand(ctx, "systemctl", "is-active", unit)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(out)
	if state == "" {
		state = "unknown"
	}
	return state, nil
}

func ServiceAction(ctx context.Context, action, unit string) error {
	action = strings.TrimSpace(action)
	unit = strings.TrimSpace(unit)
	if action == "" || unit == "" {
		return errors.New("invalid systemctl action")
	}
	_, err := runCommand(ctx, "systemctl", action, unit)
	return err
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), errMsg)
	}
	return stdout.String(), nil
}
