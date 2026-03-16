package network

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BBRStatus returns whether BBR congestion control is enabled.
func BBRStatus() (enabled bool, current string, err error) {
	data, err := os.ReadFile("/proc/sys/net/ipv4/tcp_congestion_control")
	if err != nil {
		return false, "", fmt.Errorf("read tcp_congestion_control: %w", err)
	}
	current = strings.TrimSpace(string(data))
	return current == "bbr", current, nil
}

// EnableBBR enables BBR congestion control via sysctl.
func EnableBBR() error {
	// Load TCP BBR module.
	exec.Command("modprobe", "tcp_bbr").Run()

	// Set sysctl values.
	settings := map[string]string{
		"net.core.default_qdisc":          "fq",
		"net.ipv4.tcp_congestion_control": "bbr",
	}

	for key, val := range settings {
		cmd := exec.Command("sysctl", "-w", key+"="+val)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sysctl %s=%s: %s: %s", key, val, err, string(out))
		}
	}

	// Persist in sysctl.conf.
	return persistSysctl(settings)
}

func persistSysctl(settings map[string]string) error {
	const path = "/etc/sysctl.conf"
	existing, _ := os.ReadFile(path)
	content := string(existing)

	for key, val := range settings {
		line := key + " = " + val
		if !strings.Contains(content, key) {
			content += "\n" + line
		}
	}

	return os.WriteFile(path, []byte(content), 0644)
}
