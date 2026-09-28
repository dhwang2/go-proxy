package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

const subUsage = "gproxy sub [--user <user>] [--target <domain|ip>]"

func registerSub(r *Runner, root *cobra.Command) {
	var p application.SubscriptionOptions
	// Bare, sub answers with its one form; any option runs the export.
	subArgs := func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return application.Invalid("sub takes --user <user>, not a positional argument")
		}
		if cmd.Flags().Changed("user") || cmd.Flags().Changed("target") {
			return nil
		}
		return guidance("gproxy sub requires --user or --target", []string{subUsage}, map[string]any{"usage": subUsage})
	}
	cmd := r.leaf("sub", "Export credential-bearing client links", subArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		if cmd.Flags().Changed("user") && p.User == "" {
			return application.Result{}, application.Invalid("--user cannot be empty")
		}
		p.JSON = r.JSON
		return r.App.Subscription(ctx, p)
	})
	cmd.Flags().StringVar(&p.User, "user", "", "Only this user's links; every user's when omitted")
	cmd.Flags().StringVar(&p.Target, "target", "", "Address links name: domain, the configured domain, or ip, the server's addresses; default domain")
	root.AddCommand(cmd)
}
