package core

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Component represents a managed binary component.
type Component string

const (
	CompSingBox   Component = "sing-box"
	CompSnell     Component = "snell"
	CompShadowTLS Component = "shadow-tls"
	CompCaddy     Component = "caddy"
)

// AllComponents returns all managed components.
func AllComponents() []Component {
	return []Component{CompSingBox, CompSnell, CompShadowTLS, CompCaddy}
}

// VersionInfo holds version information for a component.
type VersionInfo struct {
	Component Component `json:"component"`
	Version   string    `json:"version"`
	Installed bool      `json:"installed"`
	Error     string    `json:"error,omitempty"`
}

// DetectVersion returns the installed version of a component binary.
// It uses the provided context for timeout control on the exec call.
func DetectVersion(ctx context.Context, binPath string, component Component) VersionInfo {
	info := VersionInfo{Component: component}

	// Check binary exists first to avoid exec on missing files.
	if _, err := os.Stat(binPath); err != nil {
		if !os.IsNotExist(err) {
			info.Error = "cannot inspect binary"
		}
		return info
	}
	info.Installed = true

	// Use a per-binary timeout of 10 seconds to prevent hangs.
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch component {
	case CompSingBox:
		cmd = exec.CommandContext(execCtx, binPath, "version")
	case CompCaddy:
		cmd = exec.CommandContext(execCtx, binPath, "version")
	default:
		cmd = exec.CommandContext(execCtx, binPath, "--version")
	}

	var out []byte
	var err error
	if component == CompSnell {
		out, err = cmd.CombinedOutput()
	} else {
		out, err = cmd.Output()
	}
	if err != nil {
		info.Error = "cannot detect binary version"
		return info
	}

	info.Installed = true
	info.Version = parseVersion(string(out), component)
	return info
}

// InstalledVersion is DetectVersion plus what the binary cannot say about
// itself. The verified snell archive is 6.0.0rc2 while its executable reports
// v6.0.0, so `core version` and `core check` gave two answers about one
// installation until both read the receipt written beside the binary.
//
// The receipt only refines the version the executable reports; a receipt left
// behind by an unrelated build never replaces a version that disagrees with it.
func InstalledVersion(ctx context.Context, binPath string, component Component) VersionInfo {
	info := DetectVersion(ctx, binPath, component)
	if component != CompSnell || strings.TrimPrefix(strings.TrimSpace(info.Version), "v") != "6.0.0" {
		return info
	}
	data, err := os.ReadFile(binPath + ".version")
	if err != nil {
		return info
	}
	if recorded := strings.TrimSpace(string(data)); recorded != "" {
		info.Version = recorded
	}
	return info
}

func parseVersion(output string, component Component) string {
	output = strings.TrimSpace(output)
	switch component {
	case CompSingBox:
		// "sing-box version 1.x.x"
		if parts := strings.Fields(output); len(parts) >= 3 {
			return parts[2]
		}
	case CompCaddy:
		// "v2.x.x ..."
		if parts := strings.Fields(output); len(parts) >= 1 {
			return parts[0]
		}
	case CompShadowTLS:
		// "shadow-tls 0.2.25" -> extract last field as version
		if parts := strings.Fields(output); len(parts) >= 1 {
			return parts[len(parts)-1]
		}
	case CompSnell:
		_, version, _ := strings.Cut(output, "snell-server ")
		if parts := strings.Fields(version); len(parts) > 0 {
			return parts[0]
		}
	default:
		// First line, first word.
		lines := strings.Split(output, "\n")
		if len(lines) > 0 {
			return strings.TrimSpace(lines[0])
		}
	}
	return "unknown"
}
