package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-proxy/internal/derived"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/subscription"
	"go-proxy/internal/tui"
	"go-proxy/internal/tui/views"
	"go-proxy/internal/user"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		runTUI()
		return
	}

	switch os.Args[1] {
	case "menu":
		runTUI()
	case "version":
		fmt.Printf("go-proxy %s\n", version)
	case "status":
		cmdServiceAction("status")
	case "start":
		cmdServiceAction("start")
	case "stop":
		cmdServiceAction("stop")
	case "restart":
		cmdServiceAction("restart")
	case "config":
		cmdConfig()
	case "user":
		cmdUser()
	case "protocol":
		cmdProtocol()
	case "routing":
		cmdRouting()
	case "sub":
		cmdSub()
	case "update":
		fmt.Println("self-update not yet implemented in CLI mode")
	case "log":
		cmdLog()
	case "core":
		fmt.Println("core management not yet implemented in CLI mode")
	case "network":
		fmt.Println("network management not yet implemented in CLI mode")
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runTUI() {
	s, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	m := tui.NewModel(s, version)

	// Register all views.
	m.RegisterView(views.NewMainMenuView(&m))
	m.RegisterView(views.NewProtocolInstallView(&m))
	m.RegisterView(views.NewProtocolRemoveView(&m))
	m.RegisterView(views.NewUserView(&m))
	m.RegisterView(views.NewServiceView(&m))
	m.RegisterView(views.NewSubscriptionView(&m))
	m.RegisterView(views.NewConfigView(&m))
	m.RegisterView(views.NewRoutingView(&m))
	m.RegisterView(views.NewNetworkView(&m))
	m.RegisterView(views.NewCoreView(&m))
	m.RegisterView(views.NewLogsView(&m))
	m.RegisterView(views.NewUninstallView(&m))
	m.RegisterView(views.NewSelfUpdateView(&m))

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
}

func cmdServiceAction(action string) {
	ctx := context.Background()
	svc := service.SingBox
	if len(os.Args) > 2 {
		svc = service.Name(os.Args[2])
	}
	var err error
	switch action {
	case "start":
		err = service.Start(ctx, svc)
	case "stop":
		err = service.Stop(ctx, svc)
	case "restart":
		err = service.Restart(ctx, svc)
	case "status":
		st, e := service.GetStatus(ctx, svc)
		if e != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", e)
			os.Exit(1)
		}
		status := "inactive"
		if st.Running {
			status = "active (running)"
		}
		fmt.Printf("%s: %s\n", svc, status)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s: %v\n", action, svc, err)
		os.Exit(1)
	}
	fmt.Printf("%s %s: ok\n", action, svc)
}

func cmdLog() {
	unit := "sing-box"
	if len(os.Args) > 2 {
		unit = os.Args[2]
	}
	cmd := exec.Command("journalctl", "-u", unit, "-n", "50", "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read logs: %v\nhint: try 'sudo proxy log' or 'journalctl -u %s -f'\n", err, unit)
		os.Exit(1)
	}
}

func cmdConfig() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: proxy config view|validate")
		os.Exit(1)
	}
	sub := os.Args[2]
	switch sub {
	case "view":
		s, err := store.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(configJSON(s))
	case "validate":
		s, err := store.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := s.Apply(); err != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("config valid")
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", sub)
		os.Exit(1)
	}
}

func cmdUser() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: proxy user list|add|rename|delete")
		os.Exit(1)
	}
	s, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	switch os.Args[2] {
	case "list":
		users := user.List(s)
		for _, u := range users {
			fmt.Printf("%s  (%d protocols, %d routes)\n", u.Name, len(u.Memberships), u.RouteCount)
		}
	case "add":
		name := argOrEmpty(3)
		if name == "" {
			fmt.Fprintln(os.Stderr, "usage: proxy user add <name>")
			os.Exit(1)
		}
		if err := user.Add(s, name); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		applyOrExit(s)
		fmt.Printf("added user: %s\n", name)
	case "rename":
		old := argOrEmpty(3)
		new := argOrEmpty(4)
		if old == "" || new == "" {
			fmt.Fprintln(os.Stderr, "usage: proxy user rename <old> <new>")
			os.Exit(1)
		}
		if err := user.Rename(s, old, new); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		applyOrExit(s)
		fmt.Printf("renamed: %s → %s\n", old, new)
	case "delete":
		name := argOrEmpty(3)
		if name == "" {
			fmt.Fprintln(os.Stderr, "usage: proxy user delete <name>")
			os.Exit(1)
		}
		if err := user.Delete(s, name); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		applyOrExit(s)
		fmt.Printf("deleted user: %s\n", name)
	default:
		fmt.Fprintf(os.Stderr, "unknown user subcommand: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func cmdProtocol() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: proxy protocol list|install|remove")
		os.Exit(1)
	}
	s, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	switch os.Args[2] {
	case "list":
		inv := derived.Inventory(s)
		for _, p := range inv {
			reality := ""
			if p.HasReality {
				reality = " [Reality]"
			}
			fmt.Printf("%s  type=%s  port=%d  users=%d%s\n",
				p.Tag, p.Type, p.Port, p.UserCount, reality)
		}
	default:
		fmt.Println("protocol install/remove: use TUI mode (proxy menu)")
	}
}

func cmdRouting() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: proxy routing list|set|clear|sync-dns|test")
		os.Exit(1)
	}
	s, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	switch os.Args[2] {
	case "list":
		for _, r := range s.UserRoutes {
			users := strings.Join(r.AuthUser, ",")
			fmt.Printf("outbound=%s  users=%s  rulesets=%s\n",
				r.Outbound, users, strings.Join(r.RuleSet, ","))
		}
	case "clear":
		name := argOrEmpty(3)
		if name != "" {
			n := routing.ClearUser(s, name)
			fmt.Printf("cleared %d rules for %s\n", n, name)
		} else {
			n := routing.ClearAll(s)
			fmt.Printf("cleared %d rules\n", n)
		}
		applyOrExit(s)
	case "sync-dns":
		routing.SyncDNS(s, nil, "ipv4_only")
		routing.SyncRouteRules(s)
		applyOrExit(s)
		fmt.Println("dns and route rules synced")
	case "set":
		userName := argOrEmpty(3)
		preset := argOrEmpty(4)
		outbound := argOrEmpty(5)
		if userName == "" || preset == "" || outbound == "" {
			fmt.Fprintln(os.Stderr, "usage: proxy routing set <user> <preset> <outbound>")
			fmt.Fprintln(os.Stderr, "\npresets:")
			for _, p := range routing.BuiltinPresets() {
				fmt.Fprintf(os.Stderr, "  %s\n", p.Name)
			}
			os.Exit(1)
		}
		var matched *routing.Preset
		for _, p := range routing.BuiltinPresets() {
			if strings.EqualFold(p.Name, preset) {
				matched = &p
				break
			}
		}
		if matched == nil {
			fmt.Fprintf(os.Stderr, "unknown preset: %s\n", preset)
			os.Exit(1)
		}
		rule := routing.PresetToRule(*matched, userName, outbound)
		if err := routing.SetRule(s, userName, rule); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		routing.SyncDNS(s, nil, "ipv4_only")
		routing.SyncRouteRules(s)
		applyOrExit(s)
		fmt.Printf("set routing rule: %s → %s for %s\n", matched.Name, outbound, userName)
	case "test":
		userName := argOrEmpty(3)
		domain := argOrEmpty(4)
		if userName == "" || domain == "" {
			fmt.Fprintln(os.Stderr, "usage: proxy routing test <user> <domain>")
			os.Exit(1)
		}
		result := routing.TestDomain(s, userName, domain)
		if len(result.MatchedRules) == 0 {
			fmt.Printf("no rules match %s for user %s\n", domain, userName)
		} else {
			for _, m := range result.MatchedRules {
				fmt.Printf("match: outbound=%s  by=%s  value=%s\n", m.Outbound, m.MatchBy, m.Value)
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown routing subcommand: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func cmdSub() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: proxy sub show|target")
		os.Exit(1)
	}
	switch os.Args[2] {
	case "show":
		s, err := store.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		userName := argOrEmpty(3)
		format := subscription.FormatSurge
		if f := argOrEmpty(4); f == "singbox" {
			format = subscription.FormatSingBox
		}
		if userName == "" {
			// Show all users.
			for _, name := range derived.UserNames(s) {
				links := subscription.Render(s, name, format, "")
				for _, l := range links {
					fmt.Printf("[%s] %s\n%s\n\n", name, l.Tag, l.Content)
				}
			}
		} else {
			links := subscription.Render(s, userName, format, "")
			for _, l := range links {
				fmt.Printf("%s\n%s\n\n", l.Tag, l.Content)
			}
		}
	case "target":
		fmt.Println(subscription.DetectTarget())
	default:
		fmt.Fprintf(os.Stderr, "unknown sub subcommand: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func configJSON(s *store.Store) string {
	data, _ := json.MarshalIndent(s.SingBox, "", "  ")
	return maskCredentials(string(data))
}

// maskCredentials replaces UUID and password values with masked versions.
func maskCredentials(s string) string {
	var result strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "\"uuid\"") ||
			strings.HasPrefix(trimmed, "\"password\"") {
			// Mask the value: keep first 8 chars of the credential, replace rest.
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				val := strings.Trim(strings.TrimSuffix(strings.TrimSpace(parts[1]), ","), "\"")
				hasSuffix := strings.HasSuffix(strings.TrimSpace(parts[1]), ",")
				masked := val
				if len(val) > 8 {
					masked = val[:8] + "********"
				}
				suffix := ""
				if hasSuffix {
					suffix = ","
				}
				result.WriteString(parts[0] + ": \"" + masked + "\"" + suffix + "\n")
				continue
			}
		}
		result.WriteString(line + "\n")
	}
	return strings.TrimRight(result.String(), "\n")
}

func applyOrExit(s *store.Store) {
	if err := s.Apply(); err != nil {
		if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
			fmt.Fprintln(os.Stderr, "permission denied, try running with sudo")
		} else {
			fmt.Fprintf(os.Stderr, "apply error: %v\n", err)
		}
		os.Exit(1)
	}
}

func argOrEmpty(i int) string {
	if i < len(os.Args) {
		return os.Args[i]
	}
	return ""
}

func printUsage() {
	fmt.Println("Usage: proxy <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  menu        Launch interactive TUI")
	fmt.Println("  version     Show version")
	fmt.Println("  status      Show service status")
	fmt.Println("  start       Start services")
	fmt.Println("  stop        Stop services")
	fmt.Println("  restart     Restart services")
	fmt.Println("  log         Show logs")
	fmt.Println("  update      Self-update")
	fmt.Println("  config      Configuration management")
	fmt.Println("  user        User management")
	fmt.Println("  protocol    Protocol management")
	fmt.Println("  network     Network management")
	fmt.Println("  core        Core binary management")
	fmt.Println("  routing     Routing management")
	fmt.Println("  sub         Subscription management")
}
