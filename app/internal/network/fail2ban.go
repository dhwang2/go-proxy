package network

import (
	"fmt"
	"os/exec"
	"strings"
)

// Fail2BanStatus returns install and running state.
func Fail2BanStatus() (installed bool, running bool, err error) {
	// Check installed via systemctl cat
	if err := exec.Command("systemctl", "cat", "fail2ban").Run(); err != nil {
		return false, false, nil
	}
	// Check active
	out, _ := exec.Command("systemctl", "is-active", "fail2ban").Output()
	active := strings.TrimSpace(string(out)) == "active"
	return true, active, nil
}

// Fail2BanInstall installs fail2ban via apt.
func Fail2BanInstall() error {
	cmd := exec.Command("apt", "install", "-y", "fail2ban")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Fail2BanEnable enables and starts fail2ban.
func Fail2BanEnable() error {
	cmd := exec.Command("systemctl", "enable", "--now", "fail2ban")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
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
