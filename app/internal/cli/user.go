package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
	"go-proxy/internal/user"
)

// names validates the positional names of a user command. Argument checks are
// the right place for it: they run before the operation starts, so a name the
// rules reject never reaches it. The application validates again and does not
// trust this.
func names(count int, incomplete func() error) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return incomplete()
		}
		for _, name := range args {
			if err := user.ValidateName(name); err != nil {
				return application.Invalid(err.Error())
			}
		}
		return nil
	}
}

func registerUser(r *Runner, root *cobra.Command) {
	cmd := &cobra.Command{Use: "user", Short: "Register, rename, remove and list users"}
	list := r.leaf("list", "List users and memberships", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.UserList(ctx)
	})
	var all bool
	add := r.leaf("add <name>", "Register a user", names(1, addUserGuidance), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.UserAdd(ctx, args[0], all)
	})
	add.Flags().BoolVar(&all, "all-protocols", false, "Enroll in all existing sing-box nodes")
	rename := r.leaf("rename <old> <new>", "Rename a user and associated routes", names(2, renameUserGuidance), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.UserRename(ctx, args[0], args[1])
	})
	// remove, not delete: every other object in the tree is removed, and one
	// verb per operation is what makes the next command guessable.
	remove := r.leaf("remove <name>", "Remove a user and memberships", r.confirming(names(1, removeUserGuidance)), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.UserDelete(ctx, args[0])
	})
	cmd.AddCommand(list, add, rename, remove)
	root.AddCommand(cmd)
}
