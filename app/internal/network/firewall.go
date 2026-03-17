package network

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// OpenPort opens a port in the firewall (nftables or iptables).
func OpenPort(port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	portStr := strconv.Itoa(port)

	// Try nftables first.
	if hasNftables() {
		return nftOpenPort(portStr, proto)
	}
	// Fall back to iptables.
	return iptablesOpenPort(portStr, proto)
}

// ClosePort closes a port in the firewall.
func ClosePort(port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	portStr := strconv.Itoa(port)

	if hasNftables() {
		return nftClosePort(portStr, proto)
	}
	return iptablesClosePort(portStr, proto)
}

// ListOpenPorts returns currently open ports from iptables/nftables.
func ListOpenPorts() (string, error) {
	if hasNftables() {
		out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
		return string(out), err
	}
	out, err := exec.Command("iptables", "-L", "-n", "--line-numbers").CombinedOutput()
	return string(out), err
}

// HasNftables returns whether nftables is available on the system.
func HasNftables() bool {
	_, err := exec.LookPath("nft")
	return err == nil
}

func hasNftables() bool {
	return HasNftables()
}

func nftOpenPort(port, proto string) error {
	// Ensure table and chain exist.
	exec.Command("nft", "add", "table", "inet", "proxy-filter").Run()
	exec.Command("nft", "add", "chain", "inet", "proxy-filter", "input",
		"{ type filter hook input priority 0 ; policy accept ; }").Run()

	cmd := exec.Command("nft", "add", "rule", "inet", "proxy-filter", "input",
		proto, "dport", port, "accept")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nft add rule: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func nftClosePort(port, proto string) error {
	// List rules and find the handle for this port.
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "proxy-filter", "input").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft list: %s", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "dport "+port) && strings.Contains(line, proto) {
			// Extract handle number.
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "handle" && i+1 < len(parts) {
					handle := parts[i+1]
					return exec.Command("nft", "delete", "rule", "inet", "proxy-filter", "input", "handle", handle).Run()
				}
			}
		}
	}
	return nil
}

func iptablesOpenPort(port, proto string) error {
	cmd := exec.Command("iptables", "-I", "INPUT", "-p", proto, "--dport", port, "-j", "ACCEPT")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func iptablesClosePort(port, proto string) error {
	cmd := exec.Command("iptables", "-D", "INPUT", "-p", proto, "--dport", port, "-j", "ACCEPT")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("iptables: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
