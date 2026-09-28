package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerSub(r *Runner, root *cobra.Command) {
	var p application.SubscriptionOptions
	// An export names whose links it is: a user, a node, or both. Bare, it
	// answers with every form it takes, filled in from this host so each line
	// runs as written.
	subArgs := func(cmd *cobra.Command, args []string) error {
		if err := atMostOne("user")(cmd, args); err != nil {
			return err
		}
		if len(args) == 1 || p.Node != "" {
			return nil
		}
		return subGuidance(r.subExample(cmd.Context()))
	}
	cmd := r.leaf("sub [user]", "Export credential-bearing client links for a user or a node", subArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		if len(args) > 0 {
			p.User = args[0]
		}
		p.JSON = r.JSON
		return r.App.Subscription(ctx, p)
	})
	cmd.Flags().StringVar(&p.Node, "node", "", "Only this node's links")
	cmd.Flags().StringVar(&p.Target, "target", "", "Address clients connect to, an ip or a hostname")
	root.AddCommand(cmd)
}

// subExample is a user and one of their nodes on this host, for guidance
// that runs as written; placeholders when no node has a user.
func (r *Runner) subExample(ctx context.Context) (user, tag string) {
	user, tag = "<user>", "<tag>"
	result, err := r.App.ProtocolList(ctx)
	if err != nil {
		return user, tag
	}
	fields, _ := result.Data.(map[string]any)
	nodes, _ := fields["nodes"].([]application.ProtocolNode)
	for _, node := range nodes {
		if len(node.Users) > 0 {
			return node.Users[0], node.Tag
		}
	}
	return user, tag
}

func subGuidance(user, tag string) error {
	return guidance("gproxy sub requires a user or --node",
		[]string{
			"gproxy sub <user> [--node <tag>] [--target <ip|host>]",
			"gproxy sub " + user,
			"gproxy sub " + user + " --node " + tag,
			"gproxy sub " + user + " --target <ip|host>",
			"gproxy sub --node " + tag,
		},
		map[string]any{"missing": []string{"<user>", "--node"}})
}
