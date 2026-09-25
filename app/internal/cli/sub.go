package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
	"go-proxy/internal/subscription"
)

func registerSub(r *Runner, root *cobra.Command) {
	var p application.SubscriptionOptions
	var surge, uri, mihomo bool
	cmd := r.leaf("sub [user]", "Export credential-bearing client links or configuration", atMostOne("user"), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		if len(args) > 0 {
			p.User = args[0]
		}
		p.JSON = r.JSON
		count := 0
		for _, value := range []bool{surge, uri, mihomo, p.JSON} {
			if value {
				count++
			}
		}
		if count > 1 {
			return application.Result{}, application.Invalid("--surge, --uri, --mihomo and --json are mutually exclusive")
		}
		if surge {
			p.Format = subscription.FormatSurge
		}
		if uri {
			p.Format = subscription.FormatURI
		}
		if mihomo {
			p.Format = subscription.FormatMihomo
		}
		return r.App.Subscription(ctx, p)
	})
	cmd.Flags().BoolVar(&surge, "surge", false, "Export native Surge proxy lines")
	cmd.Flags().BoolVar(&uri, "uri", false, "Export native node URIs")
	cmd.Flags().BoolVar(&mihomo, "mihomo", false, "Export a mihomo proxies list")
	cmd.Flags().StringVar(&p.Node, "node", "", "Select one node tag")
	cmd.Flags().StringVar(&p.Target, "target", "", "Override the public hostname or IP address")
	root.AddCommand(cmd)
}
