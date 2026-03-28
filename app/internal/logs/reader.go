package logs

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go-proxy/internal/config"
)

// ServiceLogSource returns the log file path and systemd unit for a service.
func ServiceLogSource(svc string) (logFile, unit string) {
	switch svc {
	case "sing-box":
		return config.SingBoxLog, "sing-box"
	case "snell-v5":
		return config.SnellLog, "snell-v5"
	case "shadow-tls":
		return config.ShadowTLSLog, "shadow-tls"
	case "caddy-sub":
		return config.CaddySubLog, "caddy-sub"
	default:
		return "", svc
	}
}

// ReadLog tries to read a log file, falls back to journalctl.
// Returns (content, source_note).
func ReadLog(logFile, unit string, lines int) (string, string) {
	// Try log file first.
	if logFile != "" {
		if info, err := os.Stat(logFile); err == nil && info.Size() > 0 {
			content := TailFile(logFile, lines)
			if content != "" {
				return content, logFile
			}
		}
	}

	// Fall back to journalctl (--boot limits to current boot to avoid stale entries after reinstall).
	if unit != "" {
		out, err := exec.Command("journalctl", "-u", unit, "-n", fmt.Sprintf("%d", lines), "--no-pager", "--boot").CombinedOutput()
		if err == nil {
			result := strings.TrimSpace(string(out))
			if result != "" {
				return result, "journalctl -u " + unit
			}
		}
	}

	return "暂无日志", ""
}

// TailFile reads the last N lines of a file.
func TailFile(path string, n int) string {
	out, err := exec.Command("tail", "-n", fmt.Sprintf("%d", n), path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
