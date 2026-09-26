package network

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The kernel setting and the files that persist it. Variables so tests can
// point them at a fixture.
var (
	CongestionControlPath = "/proc/sys/net/ipv4/tcp_congestion_control"
	// BBRSysctlPath is gproxy's own file; enable writes it, disable and
	// uninstall remove it.
	BBRSysctlPath = "/etc/sysctl.d/90-go-proxy-bbr.conf"
	// SysctlDirs are the directories systemd-sysctl reads, in precedence
	// order: a file in an earlier directory hides one of the same name in a
	// later one. SysctlConf is read after all of them.
	SysctlDirs = []string{"/etc/sysctl.d", "/run/sysctl.d", "/usr/local/lib/sysctl.d", "/usr/lib/sysctl.d", "/lib/sysctl.d"}
	SysctlConf = "/etc/sysctl.conf"
)

const congestionKey = "net.ipv4.tcp_congestion_control"

// BBRStatus returns whether BBR congestion control is enabled.
func BBRStatus() (enabled bool, current string, err error) {
	data, err := os.ReadFile(CongestionControlPath)
	if err != nil {
		return false, "", fmt.Errorf("read tcp_congestion_control: %w", err)
	}
	current = strings.TrimSpace(string(data))
	return current == "bbr", current, nil
}

// BBRManaged reports whether gproxy's own file persists BBR.
func BBRManaged() bool {
	_, err := os.Stat(BBRSysctlPath)
	return err == nil
}

// BBRBootSources lists the files other than gproxy's that set BBR at boot,
// in the order the settings are applied. The last file to set the key wins,
// so one that sets another algorithm discards the files before it; an empty
// list means that without gproxy's file the host boots without BBR.
func BBRBootSources() []string {
	byName := map[string]string{}
	for _, dir := range SysctlDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
		for _, match := range matches {
			if _, hidden := byName[filepath.Base(match)]; !hidden {
				byName[filepath.Base(match)] = match
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]string, 0, len(names)+1)
	for _, name := range names {
		files = append(files, byName[name])
	}
	files = append(files, SysctlConf)

	sources := []string{}
	seen := map[string]bool{}
	for _, path := range files {
		if filepath.Base(path) == filepath.Base(BBRSysctlPath) {
			continue
		}
		// Debian links /etc/sysctl.d/99-sysctl.conf to /etc/sysctl.conf:
		// one file, counted once.
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || seen[resolved] {
			continue
		}
		seen[resolved] = true
		value, set := sysctlValue(path, congestionKey)
		if !set {
			continue
		}
		if value == "bbr" {
			sources = append(sources, path)
		} else {
			sources = sources[:0]
		}
	}
	return sources
}

// sysctlValue reads one key from a sysctl file: "key = value", the key
// written with dots or slashes, an optional leading "-", comments with # or ;.
func sysctlValue(path, key string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	value, set := "", false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		name, setting, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		name = strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(name), "-"), "/", ".")
		if name == key {
			value, set = strings.TrimSpace(setting), true
		}
	}
	return value, set
}

// EnableBBR enables BBR congestion control via sysctl and persists it in
// gproxy's own file.
func EnableBBR(ctx context.Context) error {
	_, _ = runCommand(ctx, "modprobe", "tcp_bbr")
	settings := map[string]string{
		"net.core.default_qdisc": "fq",
		congestionKey:            "bbr",
	}
	for _, key := range []string{"net.core.default_qdisc", congestionKey} {
		val := settings[key]
		if out, err := runCommand(ctx, "sysctl", "-w", key+"="+val); err != nil {
			return fmt.Errorf("sysctl %s=%s: %s: %s", key, val, err, string(out))
		}
	}
	content := "net.core.default_qdisc = fq\n" + congestionKey + " = bbr\n"
	return os.WriteFile(BBRSysctlPath, []byte(content), 0644)
}

// DisableBBR switches the running kernel to cubic, the kernel's default, and
// removes gproxy's file. The fq queueing discipline stays: it works with any
// algorithm. A file gproxy did not write is left alone, and BBRBootSources
// says whether it turns BBR back on at the next boot.
func DisableBBR(ctx context.Context) error {
	if out, err := runCommand(ctx, "sysctl", "-w", congestionKey+"=cubic"); err != nil {
		return fmt.Errorf("sysctl %s=cubic: %s: %s", congestionKey, err, strings.TrimSpace(string(out)))
	}
	if err := os.Remove(BBRSysctlPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", BBRSysctlPath, err)
	}
	return nil
}
