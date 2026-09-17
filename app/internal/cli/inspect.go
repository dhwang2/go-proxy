package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerInspect(r *Runner, root *cobra.Command) {
	var probe bool
	status := r.leaf("status", "Inspect local state; explicitly probe public connectivity", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.Status(ctx, probe)
	})
	status.Flags().BoolVar(&probe, "probe", false, "Probe public IPv4 and IPv6 within a shared deadline")
	root.AddCommand(status)
	configuration := &cobra.Command{Use: "config", Short: "Inspect or validate managed configuration"}
	var secrets bool
	view := r.leaf("view <sing-box|snell|shadow-tls>", "Inspect configuration (redacted by default)", cobra.ExactArgs(1), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.ConfigView(ctx, args[0], secrets)
	})
	view.Flags().BoolVar(&secrets, "show-secrets", false, "Explicitly include server credentials; protect saved output")
	configuration.AddCommand(view, r.leaf("validate", "Validate managed configuration", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.ConfigValidate(ctx)
	}))
	root.AddCommand(configuration)
}
