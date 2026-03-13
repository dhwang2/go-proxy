package sysutil

import (
	"os"
	"os/exec"
	"strings"
)

// DetectVirtualization returns the virtualization type (kvm, openvz, lxc, etc.) or "none".
func DetectVirtualization() string {
	// Try systemd-detect-virt first.
	if out, err := exec.Command("systemd-detect-virt").Output(); err == nil {
		v := strings.TrimSpace(string(out))
		if v != "" {
			return v
		}
	}

	// Fallback: check /proc/cpuinfo for hypervisor flag.
	if cpuinfo, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := strings.Split(string(cpuinfo), "\n")
		for _, line := range lines {
			if strings.Contains(line, "hypervisor") {
				return "kvm"
			}
		}
	}

	// Fallback: check /proc/1/cgroup for container indicators.
	if cgroup, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(cgroup)
		switch {
		case strings.Contains(content, "lxc"):
			return "lxc"
		case strings.Contains(content, "docker"):
			return "docker"
		case strings.Contains(content, "kubepods"):
			return "docker"
		}
	}

	return "none"
}
