package network

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type cmdRunner func(ctx context.Context, name string, args ...string) (string, error)

var runCommandFn cmdRunner = runCommand

type BBRStatus struct {
	Kernel          string
	KernelSupported bool
	Congestion      string
	Qdisc           string
	Available       string
	Enabled         bool
}

func ConfigureBBR() error {
	return nil
}

func BBRStatusInfo(ctx context.Context) (BBRStatus, error) {
	status := BBRStatus{}
	kernel, err := runCommandFn(ctx, "uname", "-r")
	if err != nil {
		status.Kernel = "unknown"
	} else {
		status.Kernel = strings.TrimSpace(kernel)
	}
	status.KernelSupported = kernelSupportsBBRRelease(status.Kernel)

	status.Congestion = readSysctl(ctx, "net.ipv4.tcp_congestion_control")
	status.Qdisc = readSysctl(ctx, "net.core.default_qdisc")
	status.Available = readSysctl(ctx, "net.ipv4.tcp_available_congestion_control")
	status.Enabled = strings.TrimSpace(status.Congestion) == "bbr" && strings.TrimSpace(status.Qdisc) == "fq"
	return status, nil
}

func EnableBBR(ctx context.Context) (BBRStatus, error) {
	_, _ = runCommandFn(ctx, "modprobe", "tcp_bbr")
	available := readSysctl(ctx, "net.ipv4.tcp_available_congestion_control")
	if !strings.Contains(" "+available+" ", " bbr ") {
		return BBRStatusInfo(ctx)
	}
	if _, err := runCommandFn(ctx, "sysctl", "-w", "net.core.default_qdisc=fq"); err != nil {
		return BBRStatusInfo(ctx)
	}
	if _, err := runCommandFn(ctx, "sysctl", "-w", "net.ipv4.tcp_congestion_control=bbr"); err != nil {
		return BBRStatusInfo(ctx)
	}
	return BBRStatusInfo(ctx)
}

func DisableBBR(ctx context.Context) (BBRStatus, error) {
	if _, err := runCommandFn(ctx, "sysctl", "-w", "net.ipv4.tcp_congestion_control=cubic"); err != nil {
		return BBRStatusInfo(ctx)
	}
	_, _ = runCommandFn(ctx, "sysctl", "-w", "net.core.default_qdisc=fq_codel")
	return BBRStatusInfo(ctx)
}

func kernelSupportsBBRRelease(release string) bool {
	release = strings.TrimSpace(release)
	if release == "" || release == "unknown" {
		return false
	}
	base := strings.SplitN(release, "-", 2)[0]
	parts := strings.Split(base, ".")
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	if major > 4 {
		return true
	}
	return major == 4 && minor >= 9
}

func readSysctl(ctx context.Context, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	out, err := runCommandFn(ctx, "sysctl", "-n", key)
	if err != nil {
		return "unknown"
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "unknown"
	}
	return out
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), errMsg)
	}
	return stdout.String(), nil
}
