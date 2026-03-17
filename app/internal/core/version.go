package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Component represents a managed binary component.
type Component string

const (
	CompSingBox   Component = "sing-box"
	CompSnell     Component = "snell-server"
	CompShadowTLS Component = "shadow-tls"
	CompCaddy     Component = "caddy"
)

// AllComponents returns all managed components.
func AllComponents() []Component {
	return []Component{CompSingBox, CompSnell, CompShadowTLS, CompCaddy}
}

// VersionInfo holds version information for a component.
type VersionInfo struct {
	Component Component
	Version   string
	Installed bool
}

// DetectVersion returns the installed version of a component binary.
// It uses the provided context for timeout control on the exec call.
func DetectVersion(ctx context.Context, binPath string, component Component) VersionInfo {
	info := VersionInfo{Component: component}

	// Check binary exists first to avoid exec on missing files.
	if _, err := os.Stat(binPath); err != nil {
		return info
	}

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

	out, err := cmd.Output()
	if err != nil {
		return info
	}

	info.Installed = true
	info.Version = parseVersion(string(out), component)
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
	default:
		// First line, first word.
		lines := strings.Split(output, "\n")
		if len(lines) > 0 {
			return strings.TrimSpace(lines[0])
		}
	}
	return fmt.Sprintf("unknown (%s)", strings.SplitN(output, "\n", 2)[0])
}
