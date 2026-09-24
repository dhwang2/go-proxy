package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerInspect(r *Runner, root *cobra.Command) {
	root.AddCommand(r.leaf("status", "Show the host dashboard", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.Status(ctx)
	}))
	configuration := &cobra.Command{Use: "config", Short: "Inspect or validate managed configuration"}
	var secrets bool
	viewArgs := func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return nil
		}
		return configGuidance(args)
	}
	view := r.leaf("view <sing-box|snell|shadow-tls>", "Inspect configuration (redacted by default)", viewArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.ConfigView(ctx, args[0], secrets)
	})
	view.Flags().BoolVar(&secrets, "show-secrets", false, "Explicitly include server credentials; protect saved output")
	configuration.AddCommand(view, r.leaf("validate", "Validate managed configuration", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.ConfigValidate(ctx)
	}))
	root.AddCommand(configuration)
}
