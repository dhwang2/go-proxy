package network

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/dhwang2/go-proxy/internal/store"
)

type FirewallBackend string

const (
	FirewallUnsupported FirewallBackend = "unsupported"
	FirewallNFT         FirewallBackend = "nft"
	FirewallIPTables    FirewallBackend = "iptables"
)

type FirewallPortRule struct {
	Proto string
	Port  int
	Label string
}

type FirewallStatus struct {
	Backend FirewallBackend
	TCP     []int
	UDP     []int
	Rules   []FirewallPortRule
}

func ConfigureFirewall() error {
	return nil
}

func FirewallStatusInfo(ctx context.Context, st *store.Store) (FirewallStatus, error) {
	rules := desiredFirewallRules(st)
	status := FirewallStatus{
		Backend: detectFirewallBackend(ctx),
		Rules:   rules,
		TCP:     collectPortsByProto(rules, "tcp"),
		UDP:     collectPortsByProto(rules, "udp"),
	}
	return status, nil
}

func ApplyFirewall(ctx context.Context, st *store.Store) (FirewallStatus, error) {
	status, err := FirewallStatusInfo(ctx, st)
	if err != nil {
		return status, err
	}
	switch status.Backend {
	case FirewallNFT:
		if err := applyNft(ctx, status.TCP, status.UDP); err != nil {
			return status, err
		}
	case FirewallIPTables:
		if err := applyIptables(ctx, status.TCP, status.UDP); err != nil {
			return status, err
		}
	default:
		return status, fmt.Errorf("no supported firewall backend detected (nft/iptables)")
	}
	return status, nil
}

func ShowFirewallRules(ctx context.Context) (string, error) {
	backend := detectFirewallBackend(ctx)
	switch backend {
	case FirewallNFT:
		out, err := runCommandFn(ctx, "nft", "list", "table", "inet", "proxy_firewall")
		if err != nil {
			return "", err
		}
		return out, nil
	case FirewallIPTables:
		out, err := runCommandFn(ctx, "iptables", "-S", "PROXY_FIREWALL_INPUT")
		if err != nil {
			return "", err
		}
		if commandExists("ip6tables") {
			if out6, err6 := runCommandFn(ctx, "ip6tables", "-S", "PROXY_FIREWALL_INPUT6"); err6 == nil {
				out += "\n" + out6
			}
		}
		return out, nil
	default:
		return "", fmt.Errorf("no supported firewall backend detected")
	}
}

func desiredFirewallRules(st *store.Store) []FirewallPortRule {
	seen := map[string]bool{}
	out := make([]FirewallPortRule, 0)
	add := func(proto string, port int, label string) {
		if port <= 0 {
			return
		}
		k := proto + "|" + strconv.Itoa(port)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, FirewallPortRule{Proto: proto, Port: port, Label: label})
	}

	if st != nil && st.Config != nil {
		for _, in := range st.Config.Inbounds {
			port := in.ListenPort
			if port <= 0 {
				continue
			}
			proto := normalizeProtocolType(in.Type)
			switch proto {
			case "vless", "trojan", "anytls":
				add("tcp", port, proto)
			case "tuic":
				add("udp", port, proto)
			case "ss":
				add("tcp", port, proto)
				add("udp", port, proto)
			default:
				add("tcp", port, proto)
				add("udp", port, proto)
			}
		}
	}
	if st != nil && st.SnellConf != nil {
		port := parseListenPort(st.SnellConf.Get("listen"))
		if port > 0 {
			add("tcp", port, "snell-v5")
			if strings.EqualFold(strings.TrimSpace(st.SnellConf.Get("udp")), "true") {
				add("udp", port, "snell-v5")
			}
		}
	}

	// Keep management ports available.
	add("tcp", detectSSHPort(), "ssh")
	add("tcp", 80, "caddy")

	sort.Slice(out, func(i, j int) bool {
		if out[i].Proto != out[j].Proto {
			return out[i].Proto < out[j].Proto
		}
		return out[i].Port < out[j].Port
	})
	return out
}

func collectPortsByProto(rules []FirewallPortRule, proto string) []int {
	ports := make([]int, 0)
	for _, r := range rules {
		if r.Proto == proto {
			ports = append(ports, r.Port)
		}
	}
	sort.Ints(ports)
	return ports
}

func detectFirewallBackend(ctx context.Context) FirewallBackend {
	if commandExists("nft") {
		if _, err := runCommandFn(ctx, "nft", "list", "tables"); err == nil {
			return FirewallNFT
		}
	}
	if commandExists("iptables") {
		if _, err := runCommandFn(ctx, "iptables", "-L"); err == nil {
			return FirewallIPTables
		}
	}
	return FirewallUnsupported
}

func applyNft(ctx context.Context, tcpPorts, udpPorts []int) error {
	tmp, err := os.CreateTemp("", "proxy-firewall-*.nft")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := fmt.Fprintf(tmp, "table inet proxy_firewall {\n  chain input {\n    type filter hook input priority -10; policy accept;\n    ct state established,related accept\n    iifname \"lo\" accept\n    ip protocol icmp accept\n    ip6 nexthdr ipv6-icmp accept\n"); err != nil {
		return err
	}
	if len(tcpPorts) > 0 {
		fmt.Fprintf(tmp, "    tcp dport { %s } accept\n", joinPorts(tcpPorts))
	}
	if len(udpPorts) > 0 {
		fmt.Fprintf(tmp, "    udp dport { %s } accept\n", joinPorts(udpPorts))
	}
	if _, err := fmt.Fprintf(tmp, "    counter drop\n  }\n}\n"); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}

	_, _ = runCommandFn(ctx, "nft", "delete", "table", "inet", "proxy_firewall")
	_, err = runCommandFn(ctx, "nft", "-f", tmp.Name())
	return err
}

func applyIptables(ctx context.Context, tcpPorts, udpPorts []int) error {
	chainV4 := "PROXY_FIREWALL_INPUT"
	_, _ = runCommandFn(ctx, "iptables", "-N", chainV4)
	if _, err := runCommandFn(ctx, "iptables", "-F", chainV4); err != nil {
		return err
	}
	_, _ = runCommandFn(ctx, "iptables", "-C", "INPUT", "-j", chainV4)
	_, _ = runCommandFn(ctx, "iptables", "-I", "INPUT", "1", "-j", chainV4)
	if _, err := runCommandFn(ctx, "iptables", "-A", chainV4, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}
	if _, err := runCommandFn(ctx, "iptables", "-A", chainV4, "-i", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}
	if _, err := runCommandFn(ctx, "iptables", "-A", chainV4, "-p", "icmp", "-j", "ACCEPT"); err != nil {
		return err
	}
	for _, p := range tcpPorts {
		if _, err := runCommandFn(ctx, "iptables", "-A", chainV4, "-p", "tcp", "--dport", strconv.Itoa(p), "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	for _, p := range udpPorts {
		if _, err := runCommandFn(ctx, "iptables", "-A", chainV4, "-p", "udp", "--dport", strconv.Itoa(p), "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	if _, err := runCommandFn(ctx, "iptables", "-A", chainV4, "-j", "DROP"); err != nil {
		return err
	}

	if commandExists("ip6tables") {
		chainV6 := "PROXY_FIREWALL_INPUT6"
		_, _ = runCommandFn(ctx, "ip6tables", "-N", chainV6)
		_, _ = runCommandFn(ctx, "ip6tables", "-F", chainV6)
		_, _ = runCommandFn(ctx, "ip6tables", "-C", "INPUT", "-j", chainV6)
		_, _ = runCommandFn(ctx, "ip6tables", "-I", "INPUT", "1", "-j", chainV6)
		_, _ = runCommandFn(ctx, "ip6tables", "-A", chainV6, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT")
		_, _ = runCommandFn(ctx, "ip6tables", "-A", chainV6, "-i", "lo", "-j", "ACCEPT")
		_, _ = runCommandFn(ctx, "ip6tables", "-A", chainV6, "-p", "ipv6-icmp", "-j", "ACCEPT")
		for _, p := range tcpPorts {
			_, _ = runCommandFn(ctx, "ip6tables", "-A", chainV6, "-p", "tcp", "--dport", strconv.Itoa(p), "-j", "ACCEPT")
		}
		for _, p := range udpPorts {
			_, _ = runCommandFn(ctx, "ip6tables", "-A", chainV6, "-p", "udp", "--dport", strconv.Itoa(p), "-j", "ACCEPT")
		}
		_, _ = runCommandFn(ctx, "ip6tables", "-A", chainV6, "-j", "DROP")
	}
	return nil
}

func joinPorts(ports []int) string {
	if len(ports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, strconv.Itoa(p))
	}
	return strings.Join(parts, ", ")
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// detectSSHPort reads /etc/ssh/sshd_config for the Port directive, falls back to 22.
func detectSSHPort() int {
	data, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		return 22
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "Port") {
			if p, err := strconv.Atoi(fields[1]); err == nil && p > 0 {
				return p
			}
		}
	}
	return 22
}

func normalizeProtocolType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "shadowsocks", "ss":
		return "ss"
	default:
		return s
	}
}

func parseListenPort(listen string) int {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return 0
	}
	parts := strings.Split(listen, ":")
	v := strings.TrimSpace(parts[len(parts)-1])
	p, _ := strconv.Atoi(v)
	if p > 0 {
		return p
	}
	return 0
}
