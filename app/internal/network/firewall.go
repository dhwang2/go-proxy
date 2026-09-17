package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

type FirewallPortSpec struct {
	Proto   string   `json:"transport"`
	Port    int      `json:"port"`
	Sources []string `json:"sources"`
}

type DesiredPortEntry struct {
	Proto    string
	Port     int
	Services []string
}

// EnsureNft ensures the nft CLI is available, installing it if necessary.
func EnsureNft(ctx context.Context) error {
	if _, err := exec.LookPath("nft"); err == nil {
		return nil
	}
	if out, err := runCommand(ctx, "apt-get", "install", "-y", "nftables"); err != nil {
		return fmt.Errorf("install nftables: %s: %s", err, strings.TrimSpace(string(out)))
	}
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("nft not found after install")
	}
	return nil
}

// ListOpenPorts returns the raw nftables ruleset.
func ListOpenPorts(ctx context.Context) (string, error) {
	out, err := runCommand(ctx, "nft", "list", "ruleset")
	return string(out), err
}

// CurrentPortEntry represents a port rule currently active in nftables.
type CurrentPortEntry struct {
	Proto  string `json:"transport"`
	Port   int    `json:"port"`
	Action string `json:"action"`
}

// CurrentFirewallPorts parses the nftables ruleset and returns active port rules.
func CurrentFirewallPorts(ctx context.Context) ([]CurrentPortEntry, error) {
	raw, err := ListOpenPorts(ctx)
	if err != nil {
		return nil, err
	}
	return parseNftPorts(raw), nil
}

func parseNftPorts(raw string) []CurrentPortEntry {
	var entries []CurrentPortEntry
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		proto, ports, action := parseNftPortLine(line)
		if proto == "" {
			continue
		}
		for _, port := range ports {
			entries = append(entries, CurrentPortEntry{Proto: proto, Port: port, Action: action})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Port != entries[j].Port {
			return entries[i].Port < entries[j].Port
		}
		return entries[i].Proto < entries[j].Proto
	})
	return entries
}

func parseNftPortLine(line string) (proto string, ports []int, action string) {
	// Match lines like: tcp dport { 22, 80, 443 } accept
	// or: udp dport 8388 accept
	for _, p := range []string{"tcp", "udp"} {
		if !strings.HasPrefix(line, p+" dport") {
			continue
		}
		proto = p
		rest := strings.TrimPrefix(line, p+" dport")
		rest = strings.TrimSpace(rest)

		// Extract action (last word).
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return "", nil, ""
		}
		action = fields[len(fields)-1]

		// Extract port set.
		if idx := strings.Index(rest, "{"); idx >= 0 {
			end := strings.Index(rest, "}")
			if end < 0 {
				return "", nil, ""
			}
			for _, tok := range strings.Split(rest[idx+1:end], ",") {
				if port, err := strconv.Atoi(strings.TrimSpace(tok)); err == nil && port > 0 {
					ports = append(ports, port)
				}
			}
		} else if len(fields) >= 2 {
			if port, err := strconv.Atoi(fields[0]); err == nil && port > 0 {
				ports = append(ports, port)
			}
		}
		return proto, ports, action
	}
	return "", nil, ""
}

// HasManagedConvergence checks if the proxy_firewall nftables table exists.
func HasManagedConvergence(ctx context.Context) bool {
	if _, err := exec.LookPath("nft"); err != nil {
		return false
	}
	_, err := runCommand(ctx, "nft", "list", "table", "inet", "proxy_firewall")
	return err == nil
}

func FirewallManaged(ctx context.Context) (bool, error) {
	if _, err := exec.LookPath("nft"); err != nil {
		return false, nil
	}
	out, err := runCommand(ctx, "nft", "-j", "list", "tables")
	if err != nil {
		return false, fmt.Errorf("inspect nftables: %w", err)
	}
	var response struct {
		Tables []struct {
			Table struct {
				Family string `json:"family"`
				Name   string `json:"name"`
			} `json:"table"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return false, fmt.Errorf("parse nftables tables: %w", err)
	}
	for _, table := range response.Tables {
		if table.Table.Family == "inet" && table.Table.Name == "proxy_firewall" {
			return true, nil
		}
	}
	return false, nil
}

type FirewallInfo struct {
	Available bool               `json:"available"`
	Managed   bool               `json:"managed"`
	Desired   []FirewallPortSpec `json:"desired"`
	Current   []CurrentPortEntry `json:"current"`
	Add       []CurrentPortEntry `json:"add"`
	Remove    []CurrentPortEntry `json:"remove"`
	Rules     string             `json:"managed_rules"`
}

func FirewallStatus(ctx context.Context, s *store.Store, bindings []service.ShadowTLSBinding) (FirewallInfo, error) {
	info := FirewallInfo{Current: []CurrentPortEntry{}, Add: []CurrentPortEntry{}, Remove: []CurrentPortEntry{}}
	var err error
	info.Desired, err = DesiredFirewallPortsWithBindings(ctx, s, bindings)
	if err != nil {
		return info, err
	}
	if _, err = exec.LookPath("nft"); err == nil {
		info.Available = true
		info.Managed, err = FirewallManaged(ctx)
		if err != nil {
			return info, err
		}
		if info.Managed {
			out, readErr := runCommand(ctx, "nft", "list", "table", "inet", "proxy_firewall")
			if readErr != nil {
				return info, fmt.Errorf("read managed firewall: %w", readErr)
			}
			info.Rules = string(out)
			info.Current = parseNftPorts(info.Rules)
		}
	}
	wanted, current := make(map[string]bool), make(map[string]bool)
	for _, p := range info.Current {
		if p.Action == "accept" {
			current[fmt.Sprintf("%s/%d", p.Proto, p.Port)] = true
		}
	}
	for _, p := range info.Desired {
		key := fmt.Sprintf("%s/%d", p.Proto, p.Port)
		wanted[key] = true
		if !current[key] {
			info.Add = append(info.Add, CurrentPortEntry{Proto: p.Proto, Port: p.Port, Action: "accept"})
		}
	}
	for _, p := range info.Current {
		if !wanted[fmt.Sprintf("%s/%d", p.Proto, p.Port)] {
			info.Remove = append(info.Remove, p)
		}
	}
	return info, ctx.Err()
}

// RemoveFirewallRules removes all nftables tables created by this application.
func RemoveFirewallRules(ctx context.Context) error {
	nft, err := exec.LookPath("nft")
	if err != nil {
		return nil
	}
	managed, err := FirewallManaged(ctx)
	if err != nil || !managed {
		return err
	}
	if out, err := runCommand(ctx, nft, "delete", "table", "inet", "proxy_firewall"); err != nil {
		return fmt.Errorf("remove managed firewall: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func DesiredFirewallPorts(ctx context.Context, s *store.Store) ([]FirewallPortSpec, error) {
	bindings, err := service.ListShadowTLSBindings(s)
	if err != nil {
		return nil, err
	}
	return DesiredFirewallPortsWithBindings(ctx, s, bindings)
}

func DesiredFirewallPortsWithBindings(ctx context.Context, s *store.Store, bindings []service.ShadowTLSBinding) ([]FirewallPortSpec, error) {
	portMap := make(map[string]*FirewallPortSpec)

	addPort := func(port int, proto, source string) {
		if port <= 0 {
			return
		}
		proto = normalizeFirewallProto(proto)
		key := fmt.Sprintf("%s/%d", proto, port)
		if spec, ok := portMap[key]; ok {
			if !containsString(spec.Sources, source) {
				spec.Sources = append(spec.Sources, source)
			}
			return
		}
		portMap[key] = &FirewallPortSpec{
			Proto:   proto,
			Port:    port,
			Sources: []string{source},
		}
	}

	protectedBackends := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		if binding.BackendProto == "ss" || binding.BackendProto == "snell" {
			protectedBackends[fmt.Sprintf("%s/%d", binding.BackendProto, binding.BackendPort)] = true
		}
	}

	for _, info := range derived.Inventory(s) {
		switch info.Type {
		case "tuic":
			addPort(info.Port, "udp", info.Type)
		case "shadowsocks":
			if !protectedBackends[fmt.Sprintf("ss/%d", info.Port)] {
				addPort(info.Port, "tcp", "ss")
			}
			addPort(info.Port, "udp", "ss")
		case store.SnellTag:
			if !protectedBackends[fmt.Sprintf("snell/%d", info.Port)] {
				addPort(info.Port, "tcp", "snell-v6")
			}
		default:
			addPort(info.Port, "tcp", info.Type)
		}
	}

	for _, binding := range bindings {
		source := "shadow-tls"
		if binding.BackendProto != "" && binding.BackendProto != "unknown" {
			source = source + "→" + binding.BackendProto
		}
		addPort(binding.ListenPort, "tcp", source)
	}

	for _, port := range CollectSSHPorts(ctx) {
		addPort(port, "tcp", "ssh")
	}

	if requiresACMEPorts() {
		addPort(80, "tcp", "caddy")
		addPort(443, "tcp", "caddy")
	}

	if s.Firewall != nil {
		for _, port := range s.Firewall.Ports {
			addPort(port.Port, port.Proto, "custom")
		}
	}

	specs := make([]FirewallPortSpec, 0, len(portMap))
	for _, spec := range portMap {
		sort.Strings(spec.Sources)
		specs = append(specs, *spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Port != specs[j].Port {
			return specs[i].Port < specs[j].Port
		}
		return specs[i].Proto < specs[j].Proto
	})
	return specs, ctx.Err()
}

func ApplyFirewallConvergence(ctx context.Context, s *store.Store) error {
	if err := EnsureNft(ctx); err != nil {
		return err
	}
	specs, err := DesiredFirewallPorts(ctx, s)
	if err != nil {
		return err
	}
	var tcpPorts []int
	var udpPorts []int
	for _, spec := range specs {
		switch spec.Proto {
		case "udp":
			udpPorts = append(udpPorts, spec.Port)
		default:
			tcpPorts = append(tcpPorts, spec.Port)
		}
	}
	return nftApplyPorts(ctx, tcpPorts, udpPorts)
}

func DescribeDesiredPorts(ctx context.Context, s *store.Store) ([]DesiredPortEntry, error) {
	specs, err := DesiredFirewallPorts(ctx, s)
	if err != nil {
		return nil, err
	}
	entries := make([]DesiredPortEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, DesiredPortEntry{
			Proto:    spec.Proto,
			Port:     spec.Port,
			Services: append([]string(nil), spec.Sources...),
		})
	}
	return entries, nil
}

func CollectSSHPorts(ctx context.Context) []int {
	seen := make(map[int]bool)
	var ports []int
	addPort := func(value string) {
		port, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || port < 1 || port > 65535 || seen[port] {
			return
		}
		seen[port] = true
		ports = append(ports, port)
	}
	if fields := strings.Fields(os.Getenv("SSH_CONNECTION")); len(fields) == 4 {
		addPort(fields[3])
	}

	if out, err := runCommand(ctx, "sshd", "-T"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "port" {
				addPort(fields[1])
			}
		}
	}
	files, _ := filepath.Glob("/etc/ssh/sshd_config.d/*.conf")
	for _, path := range append([]string{"/etc/ssh/sshd_config"}, files...) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.EqualFold(fields[0], "Port") {
				addPort(fields[1])
			}
		}
	}

	if out, err := runCommand(ctx, "ss", "-lntp"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, "sshd") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			addPort(portFromAddress(fields[3]))
		}
	}

	addPort("22")
	sort.Ints(ports)
	return ports
}

func normalizeFirewallProto(proto string) string {
	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto == "udp" {
		return "udp"
	}
	return "tcp"
}

func requiresACMEPorts() bool {
	if _, err := os.Stat(config.CaddyFile); err == nil {
		return true
	}
	if _, err := os.Stat(config.DomainFile); err == nil {
		return true
	}
	return false
}

func nftApplyPorts(ctx context.Context, tcpPorts, udpPorts []int) error {
	var builder strings.Builder
	managed, err := FirewallManaged(ctx)
	if err != nil {
		return err
	}
	if managed {
		builder.WriteString("delete table inet proxy_firewall\n")
	}
	builder.WriteString("table inet proxy_firewall {\n")
	builder.WriteString("  chain input {\n")
	builder.WriteString("    type filter hook input priority -10; policy accept;\n")
	builder.WriteString("    ct state established,related accept\n")
	builder.WriteString("    iifname \"lo\" accept\n")
	builder.WriteString("    ip protocol icmp accept\n")
	builder.WriteString("    ip6 nexthdr ipv6-icmp accept\n")
	builder.WriteString("    meta nfproto ipv6 udp sport 547 udp dport 546 accept\n")
	if len(tcpPorts) > 0 {
		builder.WriteString("    tcp dport { ")
		builder.WriteString(joinPorts(tcpPorts, ", "))
		builder.WriteString(" } accept\n")
	}
	if len(udpPorts) > 0 {
		builder.WriteString("    udp dport { ")
		builder.WriteString(joinPorts(udpPorts, ", "))
		builder.WriteString(" } accept\n")
	}
	builder.WriteString("    counter drop\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n")

	tmp, err := os.CreateTemp("", "gproxy-nft-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(builder.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if out, err := runCommand(ctx, "nft", "-f", tmp.Name()); err != nil {
		return fmt.Errorf("nft apply: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func joinPorts(ports []int, sep string) string {
	items := make([]string, 0, len(ports))
	seen := make(map[int]bool)
	for _, port := range ports {
		if seen[port] || port <= 0 {
			continue
		}
		seen[port] = true
		items = append(items, strconv.Itoa(port))
	}
	sort.Slice(items, func(i, j int) bool {
		pi, _ := strconv.Atoi(items[i])
		pj, _ := strconv.Atoi(items[j])
		return pi < pj
	})
	return strings.Join(items, sep)
}

func portFromAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if idx := strings.LastIndex(addr, ":"); idx >= 0 && idx+1 < len(addr) {
		return addr[idx+1:]
	}
	return addr
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
