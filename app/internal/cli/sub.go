package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
	"go-proxy/internal/subscription"
)

func registerSub(r *Runner, root *cobra.Command) {
	var p application.SubscriptionOptions
	var surge, singBox, uri bool
	cmd := r.leaf("sub [user]", "Export credential-bearing client links or configuration", cobra.MaximumNArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		if len(args) > 0 {
			p.User = args[0]
		}
		p.JSON = r.JSON
		count := 0
		for _, value := range []bool{surge, singBox, uri, p.JSON} {
			if value {
				count++
			}
		}
		if count > 1 {
			return application.Result{}, application.Invalid("--surge, --sing-box, --uri and --json are mutually exclusive")
		}
		if surge {
			p.Format = subscription.FormatSurge
		}
		if singBox {
			p.Format = subscription.FormatSingBox
		}
		if uri {
			p.Format = subscription.FormatURI
		}
		return r.App.Subscription(ctx, p)
	})
	cmd.Flags().BoolVar(&surge, "surge", false, "Export native Surge proxy lines")
	cmd.Flags().BoolVar(&singBox, "sing-box", false, "Export one sing-box JSON object with outbounds")
	cmd.Flags().BoolVar(&uri, "uri", false, "Export native node URIs")
	cmd.Flags().StringVar(&p.Node, "node", "", "Select one node tag")
	cmd.Flags().StringVar(&p.Target, "target", "", "Override the public hostname or IP address")
	root.AddCommand(cmd)
}
