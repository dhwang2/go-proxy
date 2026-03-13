package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type ComponentVersion struct {
	Name      string
	Path      string
	Installed bool
	Version   string
	Raw       string
	Err       error
}

type VersionOptions struct {
	WorkDir string
}

func CheckVersion(ctx context.Context, opts VersionOptions) ([]ComponentVersion, error) {
	workDir := strings.TrimSpace(opts.WorkDir)
	if workDir == "" {
		workDir = "/etc/go-proxy"
	}
	components := []struct {
		name string
		path string
		cmds [][]string
	}{
		{name: "sing-box", path: filepath.Join(workDir, "bin", "sing-box"), cmds: [][]string{{"version"}, {"-v"}}},
		{name: "snell-v5", path: filepath.Join(workDir, "bin", "snell-server"), cmds: [][]string{{"-v"}}},
		{name: "shadow-tls", path: filepath.Join(workDir, "bin", "shadow-tls"), cmds: [][]string{{"--version"}, {"-v"}, {"version"}}},
		{name: "caddy", path: filepath.Join(workDir, "bin", "caddy"), cmds: [][]string{{"version"}}},
	}
	out := make([]ComponentVersion, 0, len(components))
	for _, c := range components {
		row := ComponentVersion{Name: c.name, Path: c.path}
		if st, err := os.Stat(c.path); err != nil || st.IsDir() {
			row.Err = fmt.Errorf("not installed")
			out = append(out, row)
			continue
		}
		row.Installed = true
		raw, err := runVersionCommand(ctx, c.path, c.cmds)
		if err != nil {
			row.Err = err
			out = append(out, row)
			continue
		}
		row.Raw = strings.TrimSpace(raw)
		row.Version = extractVersion(row.Raw)
		if row.Version == "" {
			row.Version = "unknown"
		}
		out = append(out, row)
	}
	return out, nil
}

func runVersionCommand(ctx context.Context, binPath string, cmds [][]string) (string, error) {
	if len(cmds) == 0 {
		return "", fmt.Errorf("no command")
	}
	var lastErr error
	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, binPath, args...)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = strings.TrimSpace(stdout.String())
			}
			if msg == "" {
				msg = err.Error()
			}
			lastErr = fmt.Errorf("%s %s: %s", binPath, strings.Join(args, " "), msg)
			continue
		}
		text := strings.TrimSpace(stdout.String())
		if text == "" {
			text = strings.TrimSpace(stderr.String())
		}
		return text, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("failed to read version for %s", binPath)
	}
	return "", lastErr
}

func extractVersion(raw string) string {
	re := regexp.MustCompile(`(?i)v?[0-9]+(?:\.[0-9]+){1,3}`)
	m := re.FindString(strings.TrimSpace(raw))
	return strings.TrimPrefix(strings.ToLower(m), "v")
}
