package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	confpkg "github.com/dhwang2/go-proxy/internal/config"
	corepkg "github.com/dhwang2/go-proxy/internal/core"
	"github.com/dhwang2/go-proxy/internal/derived"
	netpkg "github.com/dhwang2/go-proxy/internal/network"
	protopkg "github.com/dhwang2/go-proxy/internal/protocol"
	routingpkg "github.com/dhwang2/go-proxy/internal/routing"
	"github.com/dhwang2/go-proxy/internal/service"
	"github.com/dhwang2/go-proxy/internal/store"
	subpkg "github.com/dhwang2/go-proxy/internal/subscription"
	"github.com/dhwang2/go-proxy/internal/tui"
	updatepkg "github.com/dhwang2/go-proxy/internal/update"
	userpkg "github.com/dhwang2/go-proxy/internal/user"
)

var buildVersion = "v0.0.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	argv := os.Args[1:]
	cmd := "menu"
	subArgs := []string{}
	if len(argv) > 0 {
		cmd = strings.TrimSpace(strings.ToLower(argv[0]))
	}
	if len(argv) > 1 {
		subArgs = argv[1:]
	}

	if cmd != "version" && os.Geteuid() != 0 {
		return errors.New("go-proxy must run as root (use: sudo proxy ...)")
	}

	paths := store.DefaultPaths(store.DefaultWorkDir)
	st, err := store.Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err != nil {
		return err
	}

	svc := service.NewManager(service.Options{
		WorkDir:    store.DefaultWorkDir,
		ConfigPath: st.ConfPath(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	switch cmd {
	case "menu":
		return runMenu(ctx, st, svc)
	case "status":
		return runStatus(ctx, st, svc)
	case "config":
		return runConfigCommand(ctx, st, subArgs)
	case "user":
		return runUserCommand(ctx, st, svc, subArgs)
	case "protocol", "proto":
		return runProtocolCommand(ctx, st, svc, subArgs)
	case "network":
		return runNetworkCommand(ctx, st, subArgs)
	case "core":
		return runCoreCommand(ctx, svc, subArgs)
	case "routing":
		return runRoutingCommand(ctx, st, svc, subArgs)
	case "sub", "subscription":
		return runSubscriptionCommand(st, subArgs)
	case "bootstrap":
		return runBootstrap(ctx, svc)
	case "start", "stop", "restart":
		return runServiceAction(ctx, cmd, svc)
	case "log":
		return runLogCommand(ctx, svc, subArgs)
	case "update":
		return runUpdateCommand(ctx, subArgs)
	case "version":
		printVersion(st)
		return nil
	default:
		return fmt.Errorf("unknown command %q (supported: menu|status|config|user|protocol|network|core|routing|sub|bootstrap|start|stop|restart|log|update|version)", cmd)
	}
}

func runMenu(ctx context.Context, st *store.Store, svc *service.Manager) error {
	_ = ctx
	if err := tui.Run(st, svc, buildVersion); err != nil {
		// Fallback to non-interactive status output when TTY is unavailable.
		if strings.Contains(err.Error(), "requires a TTY") {
			return runStatus(context.Background(), st, svc)
		}
		return err
	}
	return nil
}

func runStatus(ctx context.Context, st *store.Store, svc *service.Manager) error {
	statuses, err := svc.AllStatuses(ctx)
	if err != nil {
		return err
	}

	protocols := derived.InstalledProtocols(st.Config)
	sort.Strings(protocols)
	ports := derived.ListeningPorts(st.Config)

	fmt.Printf("go-proxy %s\n", buildVersion)
	fmt.Printf("  System      %s/%s\n", st.Platform.OS, st.Platform.Arch)
	fmt.Printf("  Kernel      %s\n", st.Platform.Kernel)
	fmt.Printf("  IP Stack    %s\n", st.Platform.IPStack)
	fmt.Printf("  Config      %s\n", st.ConfPath())
	fmt.Printf("  UserMeta    %s\n", st.UserMetaPath())
	fmt.Printf("  SnellConf   %s\n", st.SnellPath())
	fmt.Println()
	fmt.Println("Services")
	for _, s := range statuses {
		version := s.Version
		if version == "" {
			version = "-"
		}
		fmt.Printf("  %-16s %-10s %s\n", s.Name, s.State, version)
	}
	fmt.Println()
	fmt.Printf("  Protocols   %s\n", strings.Join(protocols, ", "))
	fmt.Printf("  Ports       %s\n", derived.FormatPorts(ports))
	fmt.Printf("  Rules       %d\n", derived.RouteRuleCount(st.Config))
	fmt.Printf("  Members     %d\n", len(derived.ComputeMemberships(st.Config, st.UserMeta)))
	return nil
}

func runBootstrap(ctx context.Context, svc *service.Manager) error {
	fmt.Println("Bootstrapping service infrastructure...")
	results, err := svc.Bootstrap(ctx)
	if err != nil {
		return err
	}
	for _, r := range results {
		line := fmt.Sprintf("  %-16s %-8s", r.Name, r.Status)
		if r.Message != "" {
			line += " (" + r.Message + ")"
		}
		fmt.Println(line)
	}
	return nil
}

func runServiceAction(ctx context.Context, action string, svc *service.Manager) error {
	results := svc.OperateAll(ctx, action)
	fmt.Printf("Action: %s\n", action)
	for _, r := range results {
		line := fmt.Sprintf("  %-16s %s", r.Name, r.Status)
		if r.Message != "" {
			line += " (" + r.Message + ")"
		}
		fmt.Println(line)
	}
	return nil
}

func runLogCommand(ctx context.Context, svc *service.Manager, args []string) error {
	lines := 50
	follow := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(strings.ToLower(args[i]))
		switch {
		case arg == "":
			continue
		case arg == "-f" || arg == "--follow":
			follow = true
		case strings.HasPrefix(arg, "--lines="):
			v := strings.TrimSpace(strings.TrimPrefix(arg, "--lines="))
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid --lines value: %s", v)
			}
			lines = n
		case arg == "-n" || arg == "--lines":
			if i+1 >= len(args) {
				return fmt.Errorf("missing value after %s", arg)
			}
			i++
			n, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid lines value: %s", args[i])
			}
			lines = n
		default:
			if n, err := strconv.Atoi(arg); err == nil && n > 0 {
				lines = n
				continue
			}
			return fmt.Errorf("unknown log arg: %s", args[i])
		}
	}

	logCtx := ctx
	if follow {
		logCtx = context.Background()
	}
	return svc.PrintLogs(logCtx, lines, follow)
}

func printVersion(st *store.Store) {
	fmt.Printf("go-proxy %s\n", buildVersion)
	fmt.Printf("store=%s usermeta=%s\n", st.ConfPath(), st.UserMetaPath())
}

func runConfigCommand(ctx context.Context, st *store.Store, args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "view", "show":
		text, err := confpkg.Pretty(st.Config)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	case "validate":
		return runConfigValidate(ctx, st, args[1:])
	default:
		return fmt.Errorf("unknown config action %q", action)
	}
}

func runConfigValidate(ctx context.Context, st *store.Store, args []string) error {
	jsonOnly := false
	for _, arg := range args {
		switch strings.TrimSpace(strings.ToLower(arg)) {
		case "":
			continue
		case "--json-only":
			jsonOnly = true
		default:
			return fmt.Errorf("unknown validate arg: %s", arg)
		}
	}

	confPath := st.ConfPath()
	payload, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return fmt.Errorf("config json invalid: %w", err)
	}
	if jsonOnly {
		fmt.Println("Config validation passed (json syntax).")
		return nil
	}

	singBoxPath := filepath.Join(store.DefaultWorkDir, "bin", "sing-box")
	if stat, err := os.Stat(singBoxPath); err != nil || stat.IsDir() {
		fmt.Printf("Config validation passed (json syntax). sing-box binary not found at %s, skipped runtime check.\n", singBoxPath)
		return nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	checkCmd := exec.CommandContext(checkCtx, singBoxPath, "check", "-c", confPath)
	out, err := checkCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sing-box check failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	fmt.Println("Config validation passed (json + sing-box check).")
	return nil
}

func printConfigUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy config view")
	fmt.Println("  proxy config validate [--json-only]")
}

func runUserCommand(ctx context.Context, st *store.Store, svc *service.Manager, args []string) error {
	if len(args) == 0 {
		printUserUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "list":
		stats := userpkg.ListGroups(st)
		if len(stats) == 0 {
			fmt.Println("No user groups found.")
			return nil
		}
		fmt.Printf("%-24s %-8s %-10s %-10s\n", "GROUP", "ACTIVE", "DISABLED", "PROTOCOLS")
		for _, row := range stats {
			fmt.Printf("%-24s %-8d %-10d %-10d\n", row.Name, row.Active, row.Disabled, row.Protocols)
		}
		return nil
	case "add":
		if len(args) != 2 {
			return fmt.Errorf("usage: proxy user add <group_name>")
		}
		result, err := userpkg.AddGroup(st, args[1])
		if err != nil {
			return err
		}
		return applyMutation(ctx, svc, st, result, "add")
	case "rename":
		if len(args) != 3 {
			return fmt.Errorf("usage: proxy user rename <old_name> <new_name>")
		}
		result, err := userpkg.RenameGroup(st, args[1], args[2])
		if err != nil {
			return err
		}
		return applyMutation(ctx, svc, st, result, "rename")
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: proxy user delete <group_name>")
		}
		result, err := userpkg.DeleteGroup(st, args[1])
		if err != nil {
			return err
		}
		return applyMutation(ctx, svc, st, result, "delete")
	default:
		return fmt.Errorf("unknown user action %q", action)
	}
}

func applyMutation(ctx context.Context, svc *service.Manager, st *store.Store, result userpkg.MutationResult, action string) error {
	if !result.Changed() {
		fmt.Printf("User %s: no changes.\n", action)
		return nil
	}
	ops := svc.ApplyStore(ctx, st)
	fmt.Printf("User %s completed: users=%d route_rules=%d dns_rules=%d\n", action, result.AffectedUsers, result.AffectedRouteRows, result.AffectedDNSRows)
	for _, op := range ops {
		line := fmt.Sprintf("  %-12s %-8s", op.Name, op.Status)
		if op.Message != "" {
			line += " (" + op.Message + ")"
		}
		fmt.Println(line)
	}
	for _, op := range ops {
		if op.Status == "failed" {
			return fmt.Errorf("apply failed on %s: %s", op.Name, op.Message)
		}
	}
	return nil
}

func printUserUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy user list")
	fmt.Println("  proxy user add <group_name>")
	fmt.Println("  proxy user rename <old_name> <new_name>")
	fmt.Println("  proxy user delete <group_name>")
}

func runProtocolCommand(ctx context.Context, st *store.Store, svc *service.Manager, args []string) error {
	if len(args) == 0 {
		printProtocolUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "list":
		rows := protopkg.List(st)
		if len(rows) == 0 {
			fmt.Println("No protocols configured.")
			return nil
		}
		fmt.Printf("%-10s %-20s %-6s %-6s %s\n", "PROTOCOL", "TAG", "PORT", "USERS", "SOURCE")
		for _, row := range rows {
			fmt.Printf("%-10s %-20s %-6d %-6d %s\n", row.Protocol, row.Tag, row.Port, row.Users, row.Source)
		}
		return nil
	case "install":
		spec, err := parseProtocolInstallArgs(args[1:])
		if err != nil {
			return err
		}
		result, err := protopkg.Install(st, spec)
		if err != nil {
			return err
		}
		return applyProtocolMutation(ctx, svc, st, result, "install")
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: proxy protocol remove <protocol|tag>")
		}
		result, err := protopkg.Remove(st, args[1])
		if err != nil {
			return err
		}
		return applyProtocolMutation(ctx, svc, st, result, "remove")
	default:
		return fmt.Errorf("unknown protocol action %q", action)
	}
}

func parseProtocolInstallArgs(args []string) (protopkg.InstallSpec, error) {
	if len(args) == 0 {
		return protopkg.InstallSpec{}, fmt.Errorf("usage: proxy protocol install <protocol> [--port <port>] [--tag <tag>] [--group <group>] [--user <id>] [--secret <secret>] [--method <method>] [--sni <sni>]")
	}
	spec := protopkg.InstallSpec{Protocol: args[0]}
	for i := 1; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "--port=") {
			p, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--port=")))
			if err != nil || p <= 0 {
				return spec, fmt.Errorf("invalid --port value: %s", arg)
			}
			spec.Port = p
			continue
		}
		if strings.HasPrefix(arg, "--tag=") {
			spec.Tag = strings.TrimSpace(strings.TrimPrefix(arg, "--tag="))
			continue
		}
		if strings.HasPrefix(arg, "--group=") {
			spec.Group = strings.TrimSpace(strings.TrimPrefix(arg, "--group="))
			continue
		}
		if strings.HasPrefix(arg, "--user=") {
			spec.UserID = strings.TrimSpace(strings.TrimPrefix(arg, "--user="))
			continue
		}
		if strings.HasPrefix(arg, "--secret=") {
			spec.Secret = strings.TrimSpace(strings.TrimPrefix(arg, "--secret="))
			continue
		}
		if strings.HasPrefix(arg, "--method=") {
			spec.Method = strings.TrimSpace(strings.TrimPrefix(arg, "--method="))
			continue
		}
		if strings.HasPrefix(arg, "--sni=") {
			spec.SNI = strings.TrimSpace(strings.TrimPrefix(arg, "--sni="))
			continue
		}

		switch arg {
		case "--port":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --port")
			}
			p, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
			if err != nil || p <= 0 {
				return spec, fmt.Errorf("invalid --port value: %s", args[i+1])
			}
			spec.Port = p
			i++
		case "--tag":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --tag")
			}
			spec.Tag = strings.TrimSpace(args[i+1])
			i++
		case "--group":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --group")
			}
			spec.Group = strings.TrimSpace(args[i+1])
			i++
		case "--user":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --user")
			}
			spec.UserID = strings.TrimSpace(args[i+1])
			i++
		case "--secret":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --secret")
			}
			spec.Secret = strings.TrimSpace(args[i+1])
			i++
		case "--method":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --method")
			}
			spec.Method = strings.TrimSpace(args[i+1])
			i++
		case "--sni":
			if i+1 >= len(args) {
				return spec, fmt.Errorf("missing value after --sni")
			}
			spec.SNI = strings.TrimSpace(args[i+1])
			i++
		default:
			return spec, fmt.Errorf("unknown install flag: %s", arg)
		}
	}
	return spec, nil
}

func applyProtocolMutation(ctx context.Context, svc *service.Manager, st *store.Store, result protopkg.MutationResult, action string) error {
	if !result.Changed() {
		fmt.Printf("Protocol %s: no changes.\n", action)
		return nil
	}
	ops := svc.ApplyStore(ctx, st)
	fmt.Printf("Protocol %s completed: added_inbounds=%d removed_inbounds=%d meta_updates=%d\n", action, result.AddedInbounds, result.RemovedInbounds, result.UpdatedMetaRows)
	for _, op := range ops {
		line := fmt.Sprintf("  %-12s %-8s", op.Name, op.Status)
		if op.Message != "" {
			line += " (" + op.Message + ")"
		}
		fmt.Println(line)
	}
	for _, op := range ops {
		if op.Status == "failed" {
			return fmt.Errorf("apply failed on %s: %s", op.Name, op.Message)
		}
	}
	return nil
}

func printProtocolUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy protocol list")
	fmt.Println("  proxy protocol install <protocol> [--port <port>] [--tag <tag>] [--group <group>] [--user <id>] [--secret <secret>] [--method <method>] [--sni <sni>]")
	fmt.Println("  proxy protocol remove <protocol|tag>")
}

func runNetworkCommand(ctx context.Context, st *store.Store, args []string) error {
	if len(args) == 0 {
		printNetworkUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "status":
		return printNetworkStatus(ctx, st)
	case "bbr":
		return runBBRCommand(ctx, args[1:])
	case "firewall", "fw":
		return runFirewallCommand(ctx, st, args[1:])
	default:
		return fmt.Errorf("unknown network action %q", action)
	}
}

func printNetworkStatus(ctx context.Context, st *store.Store) error {
	bbr, _ := netpkg.BBRStatusInfo(ctx)
	fw, _ := netpkg.FirewallStatusInfo(ctx, st)
	fmt.Printf("BBR: enabled=%t kernel_supported=%t congestion=%s qdisc=%s\n", bbr.Enabled, bbr.KernelSupported, bbr.Congestion, bbr.Qdisc)
	fmt.Printf("Firewall: backend=%s tcp_ports=%s udp_ports=%s\n", fw.Backend, joinIntList(fw.TCP), joinIntList(fw.UDP))
	return nil
}

func runBBRCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  proxy network bbr status")
		fmt.Println("  proxy network bbr enable")
		fmt.Println("  proxy network bbr disable")
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "status":
		status, err := netpkg.BBRStatusInfo(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Kernel=%s supported=%t\n", status.Kernel, status.KernelSupported)
		fmt.Printf("Congestion=%s qdisc=%s available=%s enabled=%t\n", status.Congestion, status.Qdisc, status.Available, status.Enabled)
		return nil
	case "enable":
		status, err := netpkg.EnableBBR(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("BBR enable applied: congestion=%s qdisc=%s enabled=%t\n", status.Congestion, status.Qdisc, status.Enabled)
		return nil
	case "disable":
		status, err := netpkg.DisableBBR(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("BBR disable applied: congestion=%s qdisc=%s enabled=%t\n", status.Congestion, status.Qdisc, status.Enabled)
		return nil
	default:
		return fmt.Errorf("unknown bbr action %q", action)
	}
}

func runFirewallCommand(ctx context.Context, st *store.Store, args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  proxy network firewall status")
		fmt.Println("  proxy network firewall apply")
		fmt.Println("  proxy network firewall show")
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "status":
		status, err := netpkg.FirewallStatusInfo(ctx, st)
		if err != nil {
			return err
		}
		fmt.Printf("Firewall backend=%s\n", status.Backend)
		fmt.Printf("  tcp=%s\n", joinIntList(status.TCP))
		fmt.Printf("  udp=%s\n", joinIntList(status.UDP))
		return nil
	case "apply":
		status, err := netpkg.ApplyFirewall(ctx, st)
		if err != nil {
			return err
		}
		fmt.Printf("Firewall apply completed: backend=%s tcp=%s udp=%s\n", status.Backend, joinIntList(status.TCP), joinIntList(status.UDP))
		return nil
	case "show":
		out, err := netpkg.ShowFirewallRules(ctx)
		if err != nil {
			return err
		}
		fmt.Print(strings.TrimSpace(out))
		if strings.TrimSpace(out) != "" {
			fmt.Println()
		}
		return nil
	default:
		return fmt.Errorf("unknown firewall action %q", action)
	}
}

func printNetworkUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy network status")
	fmt.Println("  proxy network bbr status|enable|disable")
	fmt.Println("  proxy network firewall status|apply|show")
}

func joinIntList(items []int) string {
	if len(items) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(items))
	for _, v := range items {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, ",")
}

func runCoreCommand(ctx context.Context, svc *service.Manager, args []string) error {
	if len(args) == 0 {
		printCoreUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "version", "versions", "status":
		rows, err := corepkg.CheckVersion(ctx, corepkg.VersionOptions{WorkDir: store.DefaultWorkDir})
		if err != nil {
			return err
		}
		fmt.Printf("%-12s %-9s %-12s %s\n", "COMPONENT", "INSTALLED", "VERSION", "NOTE")
		for _, row := range rows {
			note := ""
			if row.Err != nil {
				note = row.Err.Error()
			}
			fmt.Printf("%-12s %-9t %-12s %s\n", row.Name, row.Installed, nonEmpty(row.Version, "-"), note)
		}
		return nil
	case "check", "update":
		opts, err := parseCoreUpdateArgs(args[1:])
		if err != nil {
			return err
		}
		opts.WorkDir = store.DefaultWorkDir
		if action == "check" {
			opts.OnlyCheck = true
		}
		rows, err := corepkg.ApplyUpdates(ctx, opts)
		if err != nil {
			return err
		}
		fmt.Printf("%-12s %-8s %-12s %-12s %-9s %s\n", "COMPONENT", "UPDATE", "CURRENT", "LATEST", "APPLIED", "NOTE")
		for _, row := range rows {
			state := "no"
			if row.NeedsUpdate {
				state = "yes"
			}
			note := strings.TrimSpace(row.Message)
			if row.Err != nil {
				note = row.Err.Error()
			}
			fmt.Printf("%-12s %-8s %-12s %-12s %-9t %s\n", row.Name, state, nonEmpty(row.Current, "-"), nonEmpty(row.Latest, "-"), row.Applied, note)
		}
		if action == "update" && !opts.DryRun && hasCoreAppliedUpdates(rows) {
			// Ensure systemd service files exist for newly installed binaries.
			created, ensureErr := svc.EnsureServiceFiles(ctx)
			if ensureErr != nil {
				fmt.Printf("Warning: failed to provision service files: %v\n", ensureErr)
			} else if len(created) > 0 {
				fmt.Printf("Provisioned service files: %s\n", strings.Join(created, ", "))
			}

			fmt.Println()
			fmt.Println("Restarting services after core update...")
			results := svc.OperateAll(ctx, "restart")
			for _, op := range results {
				line := fmt.Sprintf("  %-16s %-8s", op.Name, op.Status)
				if op.Message != "" {
					line += " (" + op.Message + ")"
				}
				fmt.Println(line)
			}
			for _, op := range results {
				if op.Status == "failed" {
					return fmt.Errorf("post-update restart failed on %s: %s", op.Name, op.Message)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown core action %q", action)
	}
}

func parseCoreUpdateArgs(args []string) (corepkg.UpdateOptions, error) {
	opts := corepkg.UpdateOptions{Component: "all"}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		switch {
		case strings.HasPrefix(arg, "--component="):
			opts.Component = strings.TrimSpace(strings.TrimPrefix(arg, "--component="))
		case strings.HasPrefix(arg, "--token="):
			opts.Token = strings.TrimSpace(strings.TrimPrefix(arg, "--token="))
		case arg == "--dry-run":
			opts.DryRun = true
		case arg == "--check":
			opts.OnlyCheck = true
		case arg == "--allow-major":
			opts.AllowMajor = true
		case arg == "--component":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value after --component")
			}
			opts.Component = strings.TrimSpace(args[i+1])
			i++
		case arg == "--token":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value after --token")
			}
			opts.Token = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown core flag: %s", arg)
			}
			if opts.Component != "" && opts.Component != "all" {
				return opts, fmt.Errorf("unexpected extra arg: %s", arg)
			}
			opts.Component = arg
		}
	}
	opts.Component = strings.ToLower(strings.TrimSpace(opts.Component))
	if opts.Component == "" {
		opts.Component = "all"
	}
	return opts, nil
}

func hasCoreAppliedUpdates(rows []corepkg.ComponentUpdate) bool {
	for _, row := range rows {
		if row.Applied {
			return true
		}
	}
	return false
}

func printCoreUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy core versions")
	fmt.Println("  proxy core check [component|all] [--token <token>]")
	fmt.Println("  proxy core update [component|all] [--dry-run] [--token <token>] [--allow-major]")
}

func runUpdateCommand(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch strings.TrimSpace(strings.ToLower(args[0])) {
		case "-h", "--help", "help":
			printUpdateUsage()
			return nil
		}
	}
	opts, checkOnly, err := parseUpdateArgs(args)
	if err != nil {
		return err
	}
	if checkOnly {
		opts.DryRun = true
	}
	result, err := updatepkg.Run(ctx, opts)
	if err != nil {
		return err
	}
	fmt.Println("Self-update")
	fmt.Printf("  current=%s\n", nonEmpty(result.CurrentRef, "-"))
	fmt.Printf("  remote=%s\n", nonEmpty(result.RemoteRef, "-"))
	fmt.Printf("  needs_update=%t\n", result.NeedsUpdate)
	fmt.Printf("  updated=%t\n", result.Updated)
	if result.Message != "" {
		fmt.Printf("  message=%s\n", result.Message)
	}
	if result.BackupPath != "" {
		fmt.Printf("  backup=%s\n", result.BackupPath)
	}
	return nil
}

func parseUpdateArgs(args []string) (updatepkg.Options, bool, error) {
	opts := updatepkg.Options{
		WorkDir: store.DefaultWorkDir,
	}
	checkOnly := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(strings.ToLower(args[i]))
		if arg == "" {
			continue
		}
		switch {
		case arg == "check":
			checkOnly = true
		case arg == "--dry-run":
			opts.DryRun = true
		case arg == "--check":
			checkOnly = true
		case strings.HasPrefix(arg, "--repo="):
			opts.Repo = strings.TrimSpace(strings.TrimPrefix(args[i], "--repo="))
		case strings.HasPrefix(arg, "--branch="):
			opts.Branch = strings.TrimSpace(strings.TrimPrefix(args[i], "--branch="))
		case strings.HasPrefix(arg, "--token="):
			opts.Token = strings.TrimSpace(strings.TrimPrefix(args[i], "--token="))
		case arg == "--repo":
			if i+1 >= len(args) {
				return opts, checkOnly, fmt.Errorf("missing value after --repo")
			}
			i++
			opts.Repo = strings.TrimSpace(args[i])
		case arg == "--branch":
			if i+1 >= len(args) {
				return opts, checkOnly, fmt.Errorf("missing value after --branch")
			}
			i++
			opts.Branch = strings.TrimSpace(args[i])
		case arg == "--token":
			if i+1 >= len(args) {
				return opts, checkOnly, fmt.Errorf("missing value after --token")
			}
			i++
			opts.Token = strings.TrimSpace(args[i])
		default:
			return opts, checkOnly, fmt.Errorf("unknown update arg: %s", args[i])
		}
	}
	return opts, checkOnly, nil
}

func printUpdateUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy update [check] [--dry-run]")
	fmt.Println("  proxy update [--repo <owner/repo>] [--branch <branch>] [--token <token>]")
}

func nonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func runRoutingCommand(ctx context.Context, st *store.Store, svc *service.Manager, args []string) error {
	if len(args) == 0 {
		printRoutingUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "list":
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		rows := routingpkg.ListRules(st, target)
		if len(rows) == 0 {
			fmt.Println("No routing rules found.")
			return nil
		}
		fmt.Printf("%-4s %-18s %-24s %s\n", "#", "OUTBOUND", "AUTH_USER", "RULE_SET")
		for i, row := range rows {
			fmt.Printf("%-4d %-18s %-24s %s\n", i+1, row.Outbound, strings.Join(row.AuthUser, ","), strings.Join(row.RuleSet, ","))
		}
		return nil
	case "set":
		if len(args) != 4 {
			return fmt.Errorf("usage: proxy routing set <user> <outbound> <rule_set_csv>")
		}
		result, err := routingpkg.UpsertUserRule(st, args[1], args[2], []string{args[3]})
		if err != nil {
			return err
		}
		return applyRoutingMutation(ctx, svc, st, result, "set")
	case "clear":
		if len(args) != 2 {
			return fmt.Errorf("usage: proxy routing clear <user>")
		}
		result, err := routingpkg.ClearUserRule(st, args[1])
		if err != nil {
			return err
		}
		return applyRoutingMutation(ctx, svc, st, result, "clear")
	case "sync-dns":
		result, err := routingpkg.SyncDNS(st)
		if err != nil {
			return err
		}
		return applyRoutingMutation(ctx, svc, st, result, "sync-dns")
	case "test":
		if len(args) != 2 {
			return fmt.Errorf("usage: proxy routing test <user>")
		}
		byOutbound, total, err := routingpkg.TestUser(st, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Routing test user=%s total_rules=%d\n", args[1], total)
		if total == 0 {
			return nil
		}
		outbounds := make([]string, 0, len(byOutbound))
		for ob := range byOutbound {
			outbounds = append(outbounds, ob)
		}
		sort.Strings(outbounds)
		for _, ob := range outbounds {
			fmt.Printf("  %-18s %d\n", ob, byOutbound[ob])
		}
		return nil
	default:
		return fmt.Errorf("unknown routing action %q", action)
	}
}

func applyRoutingMutation(ctx context.Context, svc *service.Manager, st *store.Store, result routingpkg.MutationResult, action string) error {
	if !result.Changed() {
		fmt.Printf("Routing %s: no changes.\n", action)
		return nil
	}
	ops := svc.ApplyStore(ctx, st)
	fmt.Printf("Routing %s completed: route_rules=%d dns_rules=%d\n", action, result.RouteChanged, result.DNSChanged)
	for _, op := range ops {
		line := fmt.Sprintf("  %-12s %-8s", op.Name, op.Status)
		if op.Message != "" {
			line += " (" + op.Message + ")"
		}
		fmt.Println(line)
	}
	for _, op := range ops {
		if op.Status == "failed" {
			return fmt.Errorf("apply failed on %s: %s", op.Name, op.Message)
		}
	}
	return nil
}

func printRoutingUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy routing list [user]")
	fmt.Println("  proxy routing set <user> <outbound> <rule_set_csv>")
	fmt.Println("  proxy routing clear <user>")
	fmt.Println("  proxy routing sync-dns")
	fmt.Println("  proxy routing test <user>")
}

func runSubscriptionCommand(st *store.Store, args []string) error {
	if len(args) == 0 {
		printSubscriptionUsage()
		return nil
	}
	action := strings.TrimSpace(strings.ToLower(args[0]))
	switch action {
	case "show":
		return runSubscriptionShow(st, args[1:])
	case "target":
		host := ""
		if len(args) > 1 {
			switch {
			case strings.HasPrefix(args[1], "--host="):
				host = strings.TrimSpace(strings.TrimPrefix(args[1], "--host="))
			case args[1] == "--host":
				if len(args) < 3 {
					return fmt.Errorf("missing host after --host")
				}
				host = strings.TrimSpace(args[2])
			default:
				host = strings.TrimSpace(args[1])
			}
		}
		target, err := subpkg.DetectTarget(st, host)
		if err != nil {
			return err
		}
		fmt.Printf("Subscription target preferred=%s host=%s\n", target.Preferred.Family, target.Preferred.Host)
		for _, t := range target.Targets {
			fmt.Printf("  %-8s %s\n", t.Family, t.Host)
		}
		return nil
	default:
		return fmt.Errorf("unknown sub action %q", action)
	}
}

func runSubscriptionShow(st *store.Store, args []string) error {
	userFilter := ""
	host := ""
	format := subpkg.FormatAll
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		switch {
		case strings.HasPrefix(arg, "--host="):
			host = strings.TrimSpace(strings.TrimPrefix(arg, "--host="))
		case arg == "--host":
			if i+1 >= len(args) {
				return fmt.Errorf("missing host after --host")
			}
			host = strings.TrimSpace(args[i+1])
			i++
		default:
			if f, err := subpkg.ParseFormat(arg); err == nil {
				format = f
				continue
			}
			if userFilter != "" {
				return fmt.Errorf("too many positional args: %q", arg)
			}
			userFilter = arg
		}
	}

	result, err := subpkg.Render(st, subpkg.RenderOptions{
		User:   userFilter,
		Host:   host,
		Format: format,
	})
	if err != nil {
		return err
	}

	if len(result.Users) == 0 {
		fmt.Println("No subscription links found.")
		return nil
	}
	fmt.Printf("Target preferred=%s host=%s\n", result.Target.Preferred.Family, result.Target.Preferred.Host)
	if len(result.Target.Targets) > 1 {
		for _, t := range result.Target.Targets {
			fmt.Printf("  %-8s %s\n", t.Family, t.Host)
		}
	}

	if format == subpkg.FormatAll || format == subpkg.FormatSingbox {
		fmt.Println()
		fmt.Println("[Sing-box]")
		for _, user := range result.Users {
			lines := result.ByUser[user].Singbox
			if len(lines) == 0 {
				continue
			}
			fmt.Printf("user=%s\n", user)
			for _, line := range lines {
				fmt.Println(line)
			}
			payload := strings.Join(lines, "\n")
			fmt.Printf("base64=%s\n\n", base64.StdEncoding.EncodeToString([]byte(payload)))
		}
	}
	if format == subpkg.FormatAll || format == subpkg.FormatSurge {
		fmt.Println()
		fmt.Println("[Surge]")
		for _, user := range result.Users {
			lines := result.ByUser[user].Surge
			if len(lines) == 0 {
				continue
			}
			fmt.Printf("user=%s\n", user)
			for _, line := range lines {
				fmt.Println(line)
			}
			fmt.Println()
		}
	}
	return nil
}

func printSubscriptionUsage() {
	fmt.Println("Usage:")
	fmt.Println("  proxy sub show [user] [all|singbox|surge] [--host <host>]")
	fmt.Println("  proxy sub target [host]")
}
