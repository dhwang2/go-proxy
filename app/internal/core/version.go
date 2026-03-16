package core

import (
	"fmt"
	"os/exec"
	"strings"
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
func DetectVersion(binPath string, component Component) VersionInfo {
	info := VersionInfo{Component: component}

	var cmd *exec.Cmd
	switch component {
	case CompSingBox:
		cmd = exec.Command(binPath, "version")
	case CompCaddy:
		cmd = exec.Command(binPath, "version")
	default:
		cmd = exec.Command(binPath, "--version")
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
	default:
		// First line, first word.
		lines := strings.Split(output, "\n")
		if len(lines) > 0 {
			return strings.TrimSpace(lines[0])
		}
	}
	return fmt.Sprintf("unknown (%s)", strings.SplitN(output, "\n", 2)[0])
}
