package network

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Fail2BanInfo struct {
	Installed       bool     `json:"installed"`
	Running         bool     `json:"running"`
	Managed         bool     `json:"managed"`
	SSHJailEnabled  bool     `json:"ssh_jail_enabled"`
	CurrentlyBanned int      `json:"currently_banned"`
	TotalBanned     int      `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
	MaxRetry        string   `json:"max_retry"`
	BanTime         string   `json:"ban_time"`
	FindTime        string   `json:"find_time"`
}

func Fail2BanStatus(ctx context.Context) (Fail2BanInfo, error) {
	info := Fail2BanInfo{BannedIPs: []string{}}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return info, nil
	}
	info.Installed = true
	_, err := os.Stat(Fail2BanJailPath)
	info.Managed = err == nil
	out, err := runCommand(ctx, "systemctl", "show", "fail2ban.service", "--property=LoadState,ActiveState")
	if err != nil {
		return info, fmt.Errorf("inspect fail2ban service: %w", err)
	}
	info.Running = strings.Contains(string(out), "ActiveState=active")
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

const Fail2BanJailPath = "/etc/fail2ban/jail.d/go-proxy-sshd.local"

func Fail2BanEnable(ctx context.Context) error {
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		if out, err := runCommand(ctx, "apt-get", "install", "-y", "fail2ban"); err != nil {
			return fmt.Errorf("install fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if err := os.WriteFile(Fail2BanJailPath, []byte(sshdJailConfig), 0644); err != nil {
		return fmt.Errorf("write managed jail: %w", err)
	}
	if out, err := runCommand(ctx, "systemctl", "enable", "--now", "fail2ban"); err != nil {
		return fmt.Errorf("enable fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := runCommand(ctx, "fail2ban-client", "reload"); err != nil {
		return fmt.Errorf("reload fail2ban: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Fail2BanDisable(ctx context.Context) error {
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
