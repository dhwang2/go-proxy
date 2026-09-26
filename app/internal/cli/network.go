package cli

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerNetwork(r *Runner, root *cobra.Command) {
	group := &cobra.Command{Use: "network", Short: "Inspect and manage host networking", Args: cobra.NoArgs}
	root.AddCommand(group)
	// No `network status`: the addresses it listed are the dashboard's network
	// row, which looks up a NATed family's public address itself, and its ports are
	// `network firewall status`. It repeated all three and led with kernel
	// routing tables nobody could read.
	bbr := &cobra.Command{Use: "bbr", Short: "Inspect, enable or disable BBR", Args: cobra.NoArgs}
	bbrShorts := map[string]string{
		"status":  "Show the congestion control and what keeps it at boot",
		"enable":  "Switch TCP to BBR and keep it at boot",
		"disable": "Switch TCP back to cubic and drop gproxy's boot setting",
	}
	for _, action := range []string{"status", "enable", "disable"} {
		bbr.AddCommand(r.leaf(action, bbrShorts[action], cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
			return r.App.NetworkBBR(ctx, action)
		}))
	}
	group.AddCommand(bbr)
	firewall := &cobra.Command{Use: "firewall", Short: "Manage owned firewall rules and custom ports", Args: cobra.NoArgs}
	firewall.AddCommand(r.leaf("status", "Inspect desired/current ports and planned changes", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.NetworkFirewallStatus(ctx)
	}))
	firewall.AddCommand(r.leaf("apply", "Converge managed firewall rules", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.NetworkFirewallApply(ctx)
	}))
	firewall.AddCommand(r.leaf("release", "Remove only the managed firewall table", r.confirming(cobra.NoArgs), func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.NetworkFirewallClear(ctx)
	}))
	for _, action := range []string{"add", "remove"} {
		action := action
		// The port and its transport are one argument, 8443/tcp, checked here
		// rather than in the handler: a mutation must not announce itself
		// before its argument is known to be usable.
		portArgs := func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return portGuidance(action)
			}
			if len(args) > 1 {
				return application.Invalid("select one port")
			}
			_, _, err := parsePortSpec(args[0])
			return err
		}
		if action == "remove" {
			portArgs = r.confirming(portArgs)
		}
		short := "Open a custom port in the managed firewall"
		if action == "remove" {
			short = "Close a custom port in the managed firewall"
		}
		cmd := r.leaf(action+" <port>/<"+strings.Join(application.FirewallTransports(), "|")+">", short, portArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
			port, transport, err := parsePortSpec(args[0])
			if err != nil {
				return application.Result{}, err
			}
			return r.App.NetworkFirewallPort(ctx, port, transport, action == "remove")
		})
		cmd.ValidArgsFunction = completePortSpec
		firewall.AddCommand(cmd)
	}
	group.AddCommand(firewall)
	fail2ban := &cobra.Command{Use: "fail2ban", Short: "Inspect, start or stop fail2ban SSH protection", Args: cobra.NoArgs}
	shorts := map[string]string{
		"status":  "Show fail2ban, its SSH jail and the bans it holds",
		"enable":  "Start fail2ban with gproxy's SSH jail",
		"disable": "Stop fail2ban and lift its bans",
	}
	for _, action := range []string{"status", "enable", "disable"} {
		fail2ban.AddCommand(r.leaf(action, shorts[action], cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
			return r.App.NetworkFail2Ban(ctx, action)
		}))
	}
	group.AddCommand(fail2ban)
}

// parsePortSpec reads <port>/<tcp|udp|both>. The transport stays required:
// opening both protocols is a decision, not a fallback for leaving it out.
func parsePortSpec(spec string) (int, string, error) {
	number, transport, found := strings.Cut(strings.TrimSpace(spec), "/")
	if !found {
		return 0, "", application.Invalid("expected <port>/<tcp|udp|both>, such as 8443/tcp")
	}
	port, err := strconv.Atoi(number)
	if err != nil || port < 1 || port > 65535 {
		return 0, "", application.Invalid("port must be between 1 and 65535")
	}
	transport = strings.ToLower(transport)
	if !slices.Contains(application.FirewallTransports(), transport) {
		return 0, "", application.Invalid("transport must be tcp, udp or both")
	}
	return port, transport, nil
}

// completePortSpec offers the transports once a port number is typed:
// 8443<Tab> becomes 8443/tcp, 8443/udp or 8443/both.
func completePortSpec(_ *cobra.Command, args []string, typed string) ([]string, cobra.ShellCompDirective) {
	number, _, _ := strings.Cut(typed, "/")
	if len(args) > 0 || number == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if _, err := strconv.Atoi(number); err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	candidates := []string{}
	for _, transport := range application.FirewallTransports() {
		candidates = append(candidates, number+"/"+transport)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}
