package network

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Fail2BanInfo holds status information for fail2ban.
type Fail2BanInfo struct {
	Installed       bool
	Running         bool
	SSHJailEnabled  bool
	CurrentlyBanned int
	TotalBanned     int
	BannedIPs       []string
	MaxRetry        string
	BanTime         string
	FindTime        string
}

// Fail2BanStatus returns detailed fail2ban status.
func Fail2BanStatus() (Fail2BanInfo, error) {
	var info Fail2BanInfo

	// Check installed via systemctl cat
	if err := exec.Command("systemctl", "cat", "fail2ban").Run(); err != nil {
		return info, nil
	}
	info.Installed = true

	// Check active
	out, _ := exec.Command("systemctl", "is-active", "fail2ban").Output()
	info.Running = strings.TrimSpace(string(out)) == "active"

	if !info.Running {
		return info, nil
	}

	// Query sshd jail status
	jailOut, err := exec.Command("fail2ban-client", "status", "sshd").Output()
	if err != nil {
		// jail may not exist; not an error for the caller
		return info, nil
	}
	info.SSHJailEnabled = true
	parseJailStatus(&info, string(jailOut))

	// MaxRetry/BanTime/FindTime are not in "status" output; query separately.
	if out, err := exec.Command("fail2ban-client", "get", "sshd", "maxretry").Output(); err == nil {
		info.MaxRetry = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("fail2ban-client", "get", "sshd", "bantime").Output(); err == nil {
		info.BanTime = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("fail2ban-client", "get", "sshd", "findtime").Output(); err == nil {
		info.FindTime = strings.TrimSpace(string(out))
	}
	return info, nil
}

func parseJailStatus(info *Fail2BanInfo, output string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Strip leading "|- ", "|  |- ", "`- " etc.
		if idx := strings.LastIndex(line, "|- "); idx >= 0 {
			line = line[idx+3:]
		} else if idx := strings.LastIndex(line, "`- "); idx >= 0 {
			line = line[idx+3:]
		}
		switch {
		case strings.HasPrefix(line, "Currently banned:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "Currently banned:"))
			fmt.Sscanf(val, "%d", &info.CurrentlyBanned)
		case strings.HasPrefix(line, "Total banned:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "Total banned:"))
			fmt.Sscanf(val, "%d", &info.TotalBanned)
		case strings.HasPrefix(line, "Banned IP list:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "Banned IP list:"))
			if val != "" {
				info.BannedIPs = strings.Fields(val)
			}
		}
	}
}

const sshdJailConfig = `[sshd]
enabled = true
port = ssh
filter = sshd
maxretry = 5
bantime = 3600
findtime = 600
`

const sshdJailPath = "/etc/fail2ban/jail.d/sshd.local"

// Fail2BanEnable installs (if needed), configures sshd jail, and enables fail2ban.
func Fail2BanEnable() error {
	// Install if not present
	if err := exec.Command("systemctl", "cat", "fail2ban").Run(); err != nil {
		cmd := exec.Command("apt-get", "install", "-y", "fail2ban")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	// Write sshd jail config
	if err := os.WriteFile(sshdJailPath, []byte(sshdJailConfig), 0644); err != nil {
		return fmt.Errorf("write jail config: %w", err)
	}
	// Enable and start
	cmd := exec.Command("systemctl", "enable", "--now", "fail2ban")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	// Reload to pick up new jail
	_ = exec.Command("fail2ban-client", "reload").Run()
	return nil
}

// Fail2BanDisable stops and disables fail2ban.
func Fail2BanDisable() error {
	cmd := exec.Command("systemctl", "disable", "--now", "fail2ban")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
