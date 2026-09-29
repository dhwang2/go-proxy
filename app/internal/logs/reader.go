package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/pkg/textutil"
)

// maxLogBytes bounds what one read returns, so a log of very long lines
// cannot become an unbounded buffer; --lines is the reader's control.
const maxLogBytes = 1 << 20

var ErrOutputLimit = errors.New("log output exceeds 1 MiB; ask for fewer --lines")

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

func command(ctx context.Context, logFile, unit string, lines int) (*exec.Cmd, string, error) {
	if logFile != "" {
		st, err := os.Stat(logFile)
		if err != nil && !os.IsNotExist(err) {
			return nil, "", err
		}
		if err == nil && st.Size() > 0 {
			return exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(lines), "--", logFile), logFile, nil
		}
	}
	return exec.CommandContext(ctx, "journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "--boot", "--output=short-iso"), "journal", nil
}

// Read returns the last lines of a service's log, cleaned: escape sequences
// stripped and each line tidied (see tidyLine).
func Read(ctx context.Context, logFile, unit string, lines int) (string, string, error) {
	cmd, source, err := command(ctx, logFile, unit, lines)
	if err != nil {
		return "", "", err
	}
	out := &limitedBuffer{limit: maxLogBytes}
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
	// sing-box, caddy and shadow-tls colour their own logs. Stripped here, where
	// the foreign text enters, rather than in the renderer: `--json` promises to
	// carry no escape sequence under any condition, and the JSON path never
	// renders.
	text := strings.Split(textutil.CleanText(out.String()), "\n")
	for i := range text {
		text[i] = tidyLine(text[i])
	}
	return strings.Join(text, "\n"), source, nil
}

var (
	// sing-box writes its timestamp with the zone offset first
	// ("+0000 2026-09-29 06:56:33 ERROR ..."); every service runs in UTC, so
	// the offset says nothing.
	zoneOffset = regexp.MustCompile(`^[+-]\d{4} (\d{4}-\d{2}-\d{2} )`)
	// What sing-box logs before its logger is set up, while it reads its
	// configuration, comes out as "WARN[0000]": the bracket is seconds since
	// the process started, always 0000 at that point.
	startupLevel = regexp.MustCompile(`^(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL|PANIC)\[\d+\] ?`)
)

// tidyLine drops the parts of a log line that carry no information.
func tidyLine(line string) string {
	line = zoneOffset.ReplaceAllString(line, "$1")
	return startupLevel.ReplaceAllString(line, "$1 ")
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
