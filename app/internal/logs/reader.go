package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"go-proxy/internal/config"
)

var ErrOutputLimit = errors.New("log output exceeds byte limit")

func ServiceLogSource(svc string) (logFile, unit string) {
	switch svc {
	case "sing-box":
		return config.SingBoxLog, svc
	case "snell-v6":
		return config.SnellLog, svc
	case "shadow-tls":
		return config.ShadowTLSLog, svc
	case "caddy-sub":
		return config.CaddySubLog, svc
	case "proxy-watchdog":
		return config.WatchdogLog, svc
	}
	if strings.HasPrefix(svc, "shadow-tls-") {
		return filepath.Join(config.LogDir, svc+".service.log"), svc
	}
	return "", svc
}

func command(ctx context.Context, logFile, unit string, lines int, follow bool) (*exec.Cmd, string, error) {
	if logFile != "" {
		st, err := os.Stat(logFile)
		if err != nil && !os.IsNotExist(err) {
			return nil, "", err
		}
		if err == nil && st.Size() > 0 {
			args := []string{"-n", strconv.Itoa(lines)}
			if follow {
				args = append(args, "-F")
			}
			args = append(args, "--", logFile)
			return exec.CommandContext(ctx, "tail", args...), logFile, nil
		}
	}
	args := []string{"-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "--boot", "--output=short-iso"}
	if follow {
		args = append(args, "--follow")
	}
	return exec.CommandContext(ctx, "journalctl", args...), "journal", nil
}
func Read(ctx context.Context, logFile, unit string, lines, maxBytes int) (string, string, error) {
	cmd, source, err := command(ctx, logFile, unit, lines, false)
	if err != nil {
		return "", "", err
	}
	out := &limitedBuffer{limit: maxBytes}
	diagnostics := &limitedBuffer{limit: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = diagnostics
	err = cmd.Run()
	if out.exceeded {
		return "", source, ErrOutputLimit
	}
	if err != nil {
		return "", source, fmt.Errorf("read log: %w", err)
	}
	return out.String(), source, nil
}
func Follow(ctx context.Context, logFile, unit string, lines int, out io.Writer) error {
	cmd, _, err := command(ctx, logFile, unit, lines, true)
	if err != nil {
		return err
	}
	cmd.Stdout = out
	cmd.Stderr = &limitedBuffer{limit: 64 << 10}
	return cmd.Run()
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		b.exceeded = true
		return 0, ErrOutputLimit
	}
	return b.buffer.Write(p)
}
func (b *limitedBuffer) String() string { return b.buffer.String() }
