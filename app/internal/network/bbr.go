package network

import (
	"context"
	"fmt"
	"os"
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
func EnableBBR(ctx context.Context) error {
	// Load TCP BBR module.
	_, _ = runCommand(ctx, "modprobe", "tcp_bbr")

	// Set sysctl values.
	settings := map[string]string{
		"net.core.default_qdisc":          "fq",
		"net.ipv4.tcp_congestion_control": "bbr",
	}

	for _, key := range []string{"net.core.default_qdisc", "net.ipv4.tcp_congestion_control"} {
		val := settings[key]
		if out, err := runCommand(ctx, "sysctl", "-w", key+"="+val); err != nil {
			return fmt.Errorf("sysctl %s=%s: %s: %s", key, val, err, string(out))
		}
	}

	// Persist in sysctl.conf.
	return persistSysctl(settings)
}

const BBRSysctlPath = "/etc/sysctl.d/90-go-proxy-bbr.conf"

func persistSysctl(settings map[string]string) error {
	content := "net.core.default_qdisc = " + settings["net.core.default_qdisc"] + "\nnet.ipv4.tcp_congestion_control = " + settings["net.ipv4.tcp_congestion_control"] + "\n"
	return os.WriteFile(BBRSysctlPath, []byte(content), 0644)
}
