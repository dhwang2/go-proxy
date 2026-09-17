package cli

import (
	"context"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerUser(r *Runner, root *cobra.Command) {
	cmd := r.leaf("user", "List users and memberships", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.UserList(ctx)
	})
	var all bool
	add := r.leaf("add <name>", "Register a user", cobra.ExactArgs(1), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.UserAdd(ctx, args[0], all)
	})
	add.Flags().BoolVar(&all, "all-protocols", false, "Enroll in all existing sing-box nodes")
	rename := r.leaf("rename <old> <new>", "Rename a user and associated routes", cobra.ExactArgs(2), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.UserRename(ctx, args[0], args[1])
	})
	del := r.leaf("delete <name>", "Delete a user and memberships", cobra.ExactArgs(1), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		if err := r.confirm(); err != nil {
			return application.Result{}, err
		}
		return r.App.UserDelete(ctx, args[0])
	})
	cmd.AddCommand(add, rename, del)
	root.AddCommand(cmd)
}
