package network

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Fail2BanInfo struct {
	Installed       bool     `json:"installed"`
	Running         bool     `json:"running"`
	Managed         bool     `json:"managed"`
	SSHJailEnabled  bool     `json:"ssh_jail_enabled"`
	CurrentlyBanned int      `json:"currently_banned"`
	TotalBanned     int      `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
	// JailSources are the configuration files other than gproxy's own that
	// switch the sshd jail on, relative to the fail2ban directory. While any
	// is listed, removing gproxy's jail leaves the jail running.
	JailSources []string `json:"jail_sources"`
	// StartsAtBoot is the fail2ban unit being enabled in systemd.
	StartsAtBoot bool `json:"starts_at_boot"`
	// BansLifted is how many addresses were banned when disable stopped
	// fail2ban; stopping it lifts them.
	BansLifted int `json:"bans_lifted,omitempty"`
	// Change is what enable or disable did: started, added, already managed,
	// stopped or already stopped. Status leaves it empty.
	Change string `json:"change,omitempty"`
	// BannedFile is where BannedIPs were written, empty when they were not.
	BannedFile string `json:"banned_file,omitempty"`
	MaxRetry   string `json:"max_retry"`
	BanTime    string `json:"ban_time"`
	FindTime   string `json:"find_time"`
}

func Fail2BanStatus(ctx context.Context) (Fail2BanInfo, error) {
	info := Fail2BanInfo{BannedIPs: []string{}, JailSources: []string{}}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return info, nil
	}
	info.Installed = true
	_, err := os.Stat(Fail2BanJailPath)
	info.Managed = err == nil
	info.JailSources = sshdJailSources(Fail2BanConfigDir)
	out, err := runCommand(ctx, "systemctl", "show", "fail2ban.service", "--property=LoadState,ActiveState,UnitFileState")
	if err != nil {
		return info, fmt.Errorf("inspect fail2ban service: %w", err)
	}
	info.Running = strings.Contains(string(out), "ActiveState=active")
	info.StartsAtBoot = strings.Contains(string(out), "UnitFileState=enabled")
	if !info.Running {
		return info, nil
	}
	out, err = runCommand(ctx, "fail2ban-client", "status")
	if err != nil {
		return info, fmt.Errorf("inspect fail2ban jails: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if _, names, ok := strings.Cut(line, "Jail list:"); ok {
			for _, name := range strings.Split(names, ",") {
				if strings.TrimSpace(name) == "sshd" {
					info.SSHJailEnabled = true
				}
			}
		}
	}
	if !info.SSHJailEnabled {
		return info, nil
	}
	out, err = runCommand(ctx, "fail2ban-client", "status", "sshd")
	if err != nil {
		return info, fmt.Errorf("inspect sshd jail: %w", err)
	}
	parseJailStatus(&info, string(out))
	for _, query := range []struct {
		name   string
		target *string
	}{{"maxretry", &info.MaxRetry}, {"bantime", &info.BanTime}, {"findtime", &info.FindTime}} {
		out, err = runCommand(ctx, "fail2ban-client", "get", "sshd", query.name)
		if err != nil {
			return info, fmt.Errorf("inspect sshd jail %s: %w", query.name, err)
		}
		*query.target = strings.TrimSpace(string(out))
	}
	return info, nil
}

func parseJailStatus(info *Fail2BanInfo, output string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.LastIndex(line, "|- "); idx >= 0 {
			line = line[idx+3:]
		} else if idx := strings.LastIndex(line, "`- "); idx >= 0 {
			line = line[idx+3:]
		}
		switch {
		case strings.HasPrefix(line, "Currently banned:"):
			fmt.Sscanf(strings.TrimPrefix(line, "Currently banned:"), "%d", &info.CurrentlyBanned)
		case strings.HasPrefix(line, "Total banned:"):
			fmt.Sscanf(strings.TrimPrefix(line, "Total banned:"), "%d", &info.TotalBanned)
		case strings.HasPrefix(line, "Banned IP list:"):
			info.BannedIPs = strings.Fields(strings.TrimPrefix(line, "Banned IP list:"))
		}
	}
}

const sshdJailConfig = `[sshd]
enabled = true
port = ssh
filter = sshd
maxretry = 5
bantime = 86400
findtime = 600
`

// Fail2BanJailPath is gproxy's own jail file.
var Fail2BanJailPath = "/etc/fail2ban/jail.d/go-proxy-sshd.local"

// Fail2BanConfigDir is where fail2ban reads its jails from.
var Fail2BanConfigDir = "/etc/fail2ban"

// sshdJailSources lists the files, other than gproxy's, whose [sshd] section
// switches the jail on, in the order fail2ban reads them: jail.conf, then
// jail.d/*.conf, jail.local, jail.d/*.local. The last file to set enabled
// wins, so a file that switches it off discards the ones before it.
func sshdJailSources(dir string) []string {
	files := []string{filepath.Join(dir, "jail.conf")}
	for _, pattern := range []string{"jail.d/*.conf", "jail.local", "jail.d/*.local"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		sort.Strings(matches)
		files = append(files, matches...)
	}
	sources := []string{}
	for _, path := range files {
		if filepath.Base(path) == filepath.Base(Fail2BanJailPath) {
			continue
		}
		enabled, set := sshdEnabled(path)
		if !set {
			continue
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			relative = path
		}
		if enabled {
			sources = append(sources, relative)
		} else {
			sources = sources[:0]
		}
	}
	return sources
}

// sshdEnabled reads one file's [sshd] enabled setting, if it has one.
func sshdEnabled(path string) (enabled, set bool) {
	file, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer file.Close()
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "sshd" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			key, value, found = strings.Cut(line, ":")
		}
		if !found || strings.TrimSpace(key) != "enabled" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "yes", "on", "1":
			enabled, set = true, true
		case "false", "no", "off", "0":
			enabled, set = false, true
		}
	}
	return enabled, set
}

func Fail2BanEnable(ctx context.Context) error {
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		if out, err := runCommand(ctx, "apt-get", "install", "-y", "fail2ban"); err != nil {
			return fmt.Errorf("install fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if err := os.WriteFile(Fail2BanJailPath, []byte(sshdJailConfig), 0644); err != nil {
		return fmt.Errorf("write managed jail: %w", err)
	}
	out, err := runCommand(ctx, "systemctl", "show", "fail2ban.service", "--property=ActiveState", "--value")
	if err != nil {
		return fmt.Errorf("inspect fail2ban: %w", err)
	}
	running := strings.TrimSpace(string(out)) == "active"
	if out, err := runCommand(ctx, "systemctl", "enable", "--now", "fail2ban"); err != nil {
		return fmt.Errorf("enable fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// A service that was running reads the new jail on reload. One just
	// started reads it as it starts, and answers on its socket only some
	// time after systemctl returns: a reload sent then finds no server.
	if running {
		if out, err := runCommand(ctx, "fail2ban-client", "reload"); err != nil {
			return fmt.Errorf("reload fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return waitForSSHDJail(ctx, fail2banStartWait)
}

// fail2banStartWait bounds how long enable waits for a starting fail2ban to
// bring the sshd jail up. It takes under a second on a small host.
var fail2banStartWait = 15 * time.Second

// waitForSSHDJail polls until fail2ban answers for the sshd jail, or the
// bound or the context runs out.
func waitForSSHDJail(ctx context.Context, bound time.Duration) error {
	deadline := time.Now().Add(bound)
	for {
		out, err := runCommand(ctx, "fail2ban-client", "status", "sshd")
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fail2ban did not bring up the sshd jail within %s: %s", bound, lastLine(string(out)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// Fail2BanRemoveJail removes gproxy's own jail file and reloads a running
// fail2ban. uninstall uses it: fail2ban and any jail it had before gproxy stay.
func Fail2BanRemoveJail(ctx context.Context) error {
	if err := os.Remove(Fail2BanJailPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove managed jail: %w", err)
	}
	out, err := runCommand(ctx, "systemctl", "show", "fail2ban.service", "--property=ActiveState", "--value")
	if err != nil {
		return fmt.Errorf("inspect fail2ban: %w", err)
	}
	if strings.TrimSpace(string(out)) != "active" {
		return nil
	}
	if out, err := runCommand(ctx, "fail2ban-client", "reload"); err != nil {
		return fmt.Errorf("reload fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Fail2BanStop turns fail2ban off: gproxy's jail file goes, and the service is
// stopped and kept from starting at boot. Every jail stops with it, whoever
// configured it, and the bans they held are lifted. enable starts it again.
func Fail2BanStop(ctx context.Context) error {
	if err := os.Remove(Fail2BanJailPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove managed jail: %w", err)
	}
	if out, err := runCommand(ctx, "systemctl", "disable", "--now", "fail2ban"); err != nil {
		return fmt.Errorf("stop fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// lastLine is the part of fail2ban-client's output worth quoting: its log
// lines end with the error.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
