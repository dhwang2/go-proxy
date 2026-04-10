package network

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

type FirewallPortSpec struct {
	Proto   string
	Port    int
	Sources []string
}

type DesiredPortEntry struct {
	Proto    string
	Port     int
	Services []string
}

// EnsureNft ensures the nft CLI is available, installing it if necessary.
func EnsureNft() error {
	if _, err := exec.LookPath("nft"); err == nil {
		return nil
	}
	if out, err := exec.Command("apt", "install", "-y", "nftables").CombinedOutput(); err != nil {
		return fmt.Errorf("install nftables: %s: %s", err, strings.TrimSpace(string(out)))
	}
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("nft not found after install")
	}
	return nil
}

// OpenPort opens a single port via nftables.
func OpenPort(port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	if err := EnsureNft(); err != nil {
		return err
	}
	portStr := strconv.Itoa(port)
	exec.Command("nft", "add", "table", "inet", "proxy-filter").Run()
	exec.Command("nft", "add", "chain", "inet", "proxy-filter", "input",
		"{ type filter hook input priority 0 ; policy accept ; }").Run()
	if out, err := exec.Command("nft", "add", "rule", "inet", "proxy-filter", "input", proto, "dport", portStr, "accept").CombinedOutput(); err != nil {
		return fmt.Errorf("nft add rule: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ClosePort closes a single port via nftables.
func ClosePort(port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	if err := EnsureNft(); err != nil {
		return err
	}
	portStr := strconv.Itoa(port)
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "proxy-filter", "input").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft list: %s", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "dport "+portStr) && strings.Contains(line, proto) {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "handle" && i+1 < len(parts) {
					return exec.Command("nft", "delete", "rule", "inet", "proxy-filter", "input", "handle", parts[i+1]).Run()
				}
			}
		}
	}
	return nil
}

// ListOpenPorts returns the raw nftables ruleset.
func ListOpenPorts() (string, error) {
	if err := EnsureNft(); err != nil {
		return "", err
	}
	out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
	return string(out), err
}

// CurrentPortEntry represents a port rule currently active in nftables.
type CurrentPortEntry struct {
	Proto  string
	Port   int
	Action string
}

// CurrentFirewallPorts parses the nftables ruleset and returns active port rules.
func CurrentFirewallPorts() ([]CurrentPortEntry, error) {
	raw, err := ListOpenPorts()
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

// FirewallBackend returns "nftables" if nft is available (or can be installed).
func FirewallBackend() string {
	if EnsureNft() == nil {
		return "nftables"
	}
	return "unsupported"
}

// HasManagedConvergence checks if the proxy_firewall nftables table exists.
func HasManagedConvergence() bool {
	if _, err := exec.LookPath("nft"); err != nil {
		return false
	}
	return exec.Command("nft", "list", "table", "inet", "proxy_firewall").Run() == nil
}

// RemoveFirewallRules removes all nftables tables created by this application.
func RemoveFirewallRules() {
	nft, err := exec.LookPath("nft")
	if err != nil {
		return
	}
	exec.Command(nft, "delete", "table", "inet", "proxy_firewall").Run()
	exec.Command(nft, "delete", "table", "inet", "proxy-filter").Run()
}

func DesiredFirewallPorts(s *store.Store) ([]FirewallPortSpec, error) {
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

	bindings, err := service.ListShadowTLSBindings(s)
	if err != nil {
		return nil, err
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
				addPort(info.Port, "tcp", "snell-v5")
			}
			if s.SnellConf != nil && s.SnellConf.UDP {
				addPort(info.Port, "udp", "snell-v5")
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

	for _, port := range CollectSSHPorts() {
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
	return specs, nil
}

func ApplyFirewallConvergence(s *store.Store) error {
	if err := EnsureNft(); err != nil {
		return err
	}
	specs, err := DesiredFirewallPorts(s)
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
	return nftApplyPorts(tcpPorts, udpPorts)
}

func ApplyConvergence(s *store.Store) error {
	return ApplyFirewallConvergence(s)
}

func DescribeDesiredPorts(s *store.Store) ([]DesiredPortEntry, error) {
	specs, err := DesiredFirewallPorts(s)
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

func CollectSSHPorts() []int {
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

	if out, err := exec.Command("sshd", "-T").CombinedOutput(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "port" {
				addPort(fields[1])
			}
		}
	}

	if out, err := exec.Command("sh", "-lc", "grep -hE '^[[:space:]]*Port[[:space:]]+[0-9]+' /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null").CombinedOutput(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.EqualFold(fields[0], "port") {
				addPort(fields[1])
			}
		}
	}

	if out, err := exec.Command("ss", "-lntp").CombinedOutput(); err == nil {
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

func nftApplyPorts(tcpPorts, udpPorts []int) error {
	var builder strings.Builder
	if exec.Command("nft", "list", "table", "inet", "proxy_firewall").Run() == nil {
		builder.WriteString("delete table inet proxy_firewall\n")
	}
	builder.WriteString("table inet proxy_firewall {\n")
	builder.WriteString("  chain input {\n")
	builder.WriteString("    type filter hook input priority -10; policy accept;\n")
	builder.WriteString("    ct state established,related accept\n")
	builder.WriteString("    iifname \"lo\" accept\n")
	builder.WriteString("    ip protocol icmp accept\n")
	builder.WriteString("    ip6 nexthdr ipv6-icmp accept\n")
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

	if out, err := exec.Command("nft", "-f", tmp.Name()).CombinedOutput(); err != nil {
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
