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

type iptablesChainSnapshot struct {
	cmdName   string
	chainName string
	exists    bool
	positions []int
	rules     [][]string
}

// OpenPort opens a single port in the active firewall backend.
func OpenPort(port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	portStr := strconv.Itoa(port)
	if HasNftables() {
		return nftOpenPort(portStr, proto)
	}
	return iptablesOpenPort(portStr, proto)
}

// ClosePort closes a single port in the active firewall backend.
func ClosePort(port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	portStr := strconv.Itoa(port)
	if HasNftables() {
		return nftClosePort(portStr, proto)
	}
	return iptablesClosePort(portStr, proto)
}

// ListOpenPorts returns the raw firewall rules from the active backend.
func ListOpenPorts() (string, error) {
	if HasNftables() {
		out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
		return string(out), err
	}
	out, err := exec.Command("iptables", "-L", "-n", "--line-numbers").CombinedOutput()
	return string(out), err
}

func FirewallBackend() string {
	if HasNftables() {
		return "nftables"
	}
	if HasIptables() {
		return "iptables"
	}
	return "unsupported"
}

// HasNftables returns whether nftables is available and usable.
func HasNftables() bool {
	_, err := exec.LookPath("nft")
	return err == nil
}

func HasIptables() bool {
	return hasIptables("iptables")
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
		for _, port := range s.Firewall.TCP {
			addPort(port, "tcp", "custom")
		}
		for _, port := range s.Firewall.UDP {
			addPort(port, "udp", "custom")
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

	switch FirewallBackend() {
	case "nftables":
		return nftApplyPorts(tcpPorts, udpPorts)
	case "iptables":
		return iptablesApplyPorts(tcpPorts, udpPorts)
	default:
		return fmt.Errorf("未检测到可用的 nftables/iptables")
	}
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

func HasManagedConvergence() bool {
	if HasNftables() && exec.Command("nft", "list", "table", "inet", "proxy_firewall").Run() == nil {
		return true
	}
	if hasIptables("iptables") && exec.Command("iptables", "-S", "PROXY_FIREWALL_INPUT").Run() == nil {
		return true
	}
	if hasIptables("ip6tables") && exec.Command("ip6tables", "-S", "PROXY_FIREWALL_INPUT6").Run() == nil {
		return true
	}
	return false
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

func nftOpenPort(port, proto string) error {
	exec.Command("nft", "add", "table", "inet", "proxy-filter").Run()
	exec.Command("nft", "add", "chain", "inet", "proxy-filter", "input",
		"{ type filter hook input priority 0 ; policy accept ; }").Run()
	cmd := exec.Command("nft", "add", "rule", "inet", "proxy-filter", "input", proto, "dport", port, "accept")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nft add rule: %s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func nftClosePort(port, proto string) error {
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "proxy-filter", "input").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft list: %s", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "dport "+port) && strings.Contains(line, proto) {
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

func iptablesApplyPorts(tcpPorts, udpPorts []int) error {
	snap4, err := captureIPTablesChain("iptables", "PROXY_FIREWALL_INPUT")
	if err != nil {
		return err
	}
	var snap6 *iptablesChainSnapshot
	if hasIptables("ip6tables") {
		captured, err := captureIPTablesChain("ip6tables", "PROXY_FIREWALL_INPUT6")
		if err != nil {
			return err
		}
		snap6 = &captured
	}

	if err := applySingleIPTablesChain("iptables", "PROXY_FIREWALL_INPUT", tcpPorts, udpPorts, "icmp"); err != nil {
		_ = restoreIPTablesChain(snap4)
		return err
	}
	if snap6 != nil {
		if err := applySingleIPTablesChain("ip6tables", "PROXY_FIREWALL_INPUT6", tcpPorts, udpPorts, "ipv6-icmp"); err != nil {
			_ = restoreIPTablesChain(*snap6)
			_ = restoreIPTablesChain(snap4)
			return err
		}
	}
	return nil
}

func applySingleIPTablesChain(cmdName, chainName string, tcpPorts, udpPorts []int, icmpProto string) error {
	tmpChain := chainName + "_NEW"
	restoreTmp := true
	defer func() {
		if restoreTmp {
			_ = cleanupIPTablesChain(cmdName, tmpChain)
		}
	}()

	_ = cleanupIPTablesChain(cmdName, tmpChain)
	if out, err := exec.Command(cmdName, "-N", tmpChain).CombinedOutput(); err != nil {
		return fmt.Errorf("%s create %s: %s: %s", cmdName, tmpChain, err, strings.TrimSpace(string(out)))
	}
	if err := appendRule(cmdName, tmpChain, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := appendRule(cmdName, tmpChain, "-i", "lo", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := appendRule(cmdName, tmpChain, "-p", icmpProto, "-j", "ACCEPT"); err != nil {
		return err
	}
	for _, port := range tcpPorts {
		if err := appendRule(cmdName, tmpChain, "-p", "tcp", "--dport", strconv.Itoa(port), "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	for _, port := range udpPorts {
		if err := appendRule(cmdName, tmpChain, "-p", "udp", "--dport", strconv.Itoa(port), "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	if err := appendRule(cmdName, tmpChain, "-j", "DROP"); err != nil {
		return err
	}
	if _, err := exec.Command(cmdName, "-C", "INPUT", "-j", tmpChain).CombinedOutput(); err != nil {
		if out, err := exec.Command(cmdName, "-I", "INPUT", "1", "-j", tmpChain).CombinedOutput(); err != nil {
			return fmt.Errorf("%s attach %s: %s: %s", cmdName, tmpChain, err, strings.TrimSpace(string(out)))
		}
	}
	for exec.Command(cmdName, "-C", "INPUT", "-j", chainName).Run() == nil {
		if out, err := exec.Command(cmdName, "-D", "INPUT", "-j", chainName).CombinedOutput(); err != nil {
			return fmt.Errorf("%s detach %s: %s: %s", cmdName, chainName, err, strings.TrimSpace(string(out)))
		}
	}
	exec.Command(cmdName, "-F", chainName).Run()
	exec.Command(cmdName, "-X", chainName).Run()
	if out, err := exec.Command(cmdName, "-E", tmpChain, chainName).CombinedOutput(); err != nil {
		return fmt.Errorf("%s rename %s: %s: %s", cmdName, tmpChain, err, strings.TrimSpace(string(out)))
	}
	if _, err := exec.Command(cmdName, "-C", "INPUT", "-j", chainName).CombinedOutput(); err != nil {
		if out, err := exec.Command(cmdName, "-I", "INPUT", "1", "-j", chainName).CombinedOutput(); err != nil {
			return fmt.Errorf("%s attach %s: %s: %s", cmdName, chainName, err, strings.TrimSpace(string(out)))
		}
	}
	for exec.Command(cmdName, "-C", "INPUT", "-j", tmpChain).Run() == nil {
		exec.Command(cmdName, "-D", "INPUT", "-j", tmpChain).Run()
	}
	restoreTmp = false
	return nil
}

func captureIPTablesChain(cmdName, chainName string) (iptablesChainSnapshot, error) {
	snapshot := iptablesChainSnapshot{
		cmdName:   cmdName,
		chainName: chainName,
	}
	out, err := exec.Command(cmdName, "-S", chainName).CombinedOutput()
	if err == nil {
		snapshot.exists = true
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "-N ") {
				continue
			}
			snapshot.rules = append(snapshot.rules, strings.Fields(line))
		}
	} else {
		msg := strings.TrimSpace(string(out))
		if !strings.Contains(msg, "No chain/target/match by that name") &&
			!strings.Contains(msg, "No chain/target/match") &&
			msg != "" {
			return snapshot, fmt.Errorf("%s snapshot %s: %s: %s", cmdName, chainName, err, msg)
		}
	}
	positions, err := captureIPTablesJumpPositions(cmdName, chainName)
	if err != nil {
		return snapshot, err
	}
	snapshot.positions = positions
	return snapshot, nil
}

func restoreIPTablesChain(snapshot iptablesChainSnapshot) error {
	if err := cleanupIPTablesChain(snapshot.cmdName, snapshot.chainName+"_NEW"); err != nil {
		return err
	}
	if err := detachIPTablesJump(snapshot.cmdName, snapshot.chainName); err != nil {
		return err
	}
	if err := cleanupIPTablesChain(snapshot.cmdName, snapshot.chainName); err != nil {
		return err
	}
	if !snapshot.exists {
		return nil
	}
	if out, err := exec.Command(snapshot.cmdName, "-N", snapshot.chainName).CombinedOutput(); err != nil {
		return fmt.Errorf("%s restore create %s: %s: %s", snapshot.cmdName, snapshot.chainName, err, strings.TrimSpace(string(out)))
	}
	for _, rule := range snapshot.rules {
		if out, err := exec.Command(snapshot.cmdName, rule...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s restore %s: %s: %s", snapshot.cmdName, strings.Join(rule, " "), err, strings.TrimSpace(string(out)))
		}
	}
	if err := attachIPTablesJumpPositions(snapshot.cmdName, snapshot.chainName, snapshot.positions); err != nil {
		return err
	}
	return nil
}

func captureIPTablesJumpPositions(cmdName, chainName string) ([]int, error) {
	out, err := exec.Command(cmdName, "-S", "INPUT").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s snapshot INPUT: %s: %s", cmdName, err, strings.TrimSpace(string(out)))
	}
	var positions []int
	position := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-A INPUT ") {
			continue
		}
		position++
		fields := strings.Fields(line)
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "-j" && fields[i+1] == chainName {
				positions = append(positions, position)
				break
			}
		}
	}
	return positions, nil
}

func attachIPTablesJumpPositions(cmdName, chainName string, positions []int) error {
	if len(positions) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(sort.IntSlice(positions)))
	for _, position := range positions {
		if position < 1 {
			position = 1
		}
		if out, err := exec.Command(cmdName, "-I", "INPUT", strconv.Itoa(position), "-j", chainName).CombinedOutput(); err != nil {
			return fmt.Errorf("%s restore attach %s@%d: %s: %s", cmdName, chainName, position, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func cleanupIPTablesChain(cmdName, chainName string) error {
	if err := detachIPTablesJump(cmdName, chainName); err != nil {
		return err
	}
	exec.Command(cmdName, "-F", chainName).Run()
	exec.Command(cmdName, "-X", chainName).Run()
	return nil
}

func detachIPTablesJump(cmdName, chainName string) error {
	for exec.Command(cmdName, "-C", "INPUT", "-j", chainName).Run() == nil {
		if out, err := exec.Command(cmdName, "-D", "INPUT", "-j", chainName).CombinedOutput(); err != nil {
			return fmt.Errorf("%s detach %s: %s: %s", cmdName, chainName, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func appendRule(cmdName, chainName string, args ...string) error {
	fullArgs := append([]string{"-A", chainName}, args...)
	if out, err := exec.Command(cmdName, fullArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %s: %s", cmdName, strings.Join(fullArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func hasIptables(cmdName string) bool {
	_, err := exec.LookPath(cmdName)
	return err == nil
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
