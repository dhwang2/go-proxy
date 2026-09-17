package cli

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerNetwork(r *Runner, root *cobra.Command) {
	group := &cobra.Command{Use: "network", Short: "Inspect and manage host networking", Args: cobra.NoArgs}
	root.AddCommand(group)
	status := r.leaf("status", "Inspect local addresses, routes and proxy ports", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, _ []string) (application.Result, error) {
		probe, _ := cmd.Flags().GetBool("probe")
		return r.App.NetworkStatus(ctx, probe)
	})
	status.Flags().Bool("probe", false, "Probe public IPv4 and IPv6 connectivity")
	group.AddCommand(status)
	bbr := &cobra.Command{Use: "bbr", Short: "Inspect or enable BBR", Args: cobra.NoArgs}
	for _, action := range []string{"status", "enable"} {
		bbr.AddCommand(r.leaf(action, "Inspect or enable BBR", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
			return r.App.NetworkBBR(ctx, action == "enable")
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
	firewall.AddCommand(r.leaf("clear", "Remove only the managed firewall table", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		if err := r.confirm(); err != nil {
			return application.Result{}, err
		}
		return r.App.NetworkFirewallClear(ctx)
	}))
	for _, action := range []string{"add", "remove"} {
		cmd := r.leaf(action+" <port>", "Persist a custom port and reconcile managed rules", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
			port, err := strconv.Atoi(args[0])
			if err != nil || port < 1 || port > 65535 {
				return application.Result{}, application.Invalid("port must be between 1 and 65535")
			}
			transport, _ := cmd.Flags().GetString("transport")
			if transport != "tcp" && transport != "udp" && transport != "both" {
				return application.Result{}, application.Invalid("transport must be tcp, udp or both")
			}
			if action == "remove" {
				if err := r.confirm(); err != nil {
					return application.Result{}, err
				}
			}
			return r.App.NetworkFirewallPort(ctx, port, transport, action == "remove")
		})
		cmd.Flags().String("transport", "", "Port transport: tcp, udp or both")
		_ = cmd.MarkFlagRequired("transport")
		firewall.AddCommand(cmd)
	}
	group.AddCommand(firewall)
	fail2ban := &cobra.Command{Use: "fail2ban", Short: "Manage owned SSH jail protection", Args: cobra.NoArgs}
	for _, action := range []string{"status", "enable", "disable"} {
		fail2ban.AddCommand(r.leaf(action, "Inspect or change managed SSH jail protection", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
			return r.App.NetworkFail2Ban(ctx, action)
		}))
	}
	group.AddCommand(fail2ban)
}
