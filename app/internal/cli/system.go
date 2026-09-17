package cli

import (
	"context"
	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerSystem(r *Runner, root *cobra.Command) {
	root.AddCommand(r.leaf("init", "Initialize runtime files and dependencies", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.Init(ctx)
	}))
	for _, action := range []string{"start", "stop", "restart"} {
		action := action
		var all bool
		cmd := r.leaf(action+" [service]", "Change managed service state", cobra.MaximumNArgs(1), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
			selector := ""
			if len(args) > 0 {
				selector = args[0]
			}
			return r.App.ServiceAction(ctx, action, selector, all)
		})
		cmd.Flags().BoolVar(&all, "all", false, "Select all installed managed services")
		root.AddCommand(cmd)
	}
	certificates := &cobra.Command{Use: "cert", Short: "Manage TLS certificates"}
	certificates.AddCommand(r.leaf("status", "Inspect the configured certificate", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.CertificateStatus(ctx)
	}))
	var domain, email string
	ensure := r.leaf("ensure", "Ensure a valid domain certificate", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.CertificateEnsure(ctx, domain, email)
	})
	ensure.Flags().StringVar(&domain, "domain", "", "Certificate domain")
	ensure.Flags().StringVar(&email, "email", "", "ACME contact email")
	certificates.AddCommand(ensure)
	root.AddCommand(certificates)
	cores := r.leaf("core", "Inspect installed core versions", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.CoreVersions(ctx)
	})
	cores.AddCommand(r.leaf("check [component]", "Check available core updates", cobra.MaximumNArgs(1), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		selector := ""
		if len(args) > 0 {
			selector = args[0]
		}
		return r.App.CoreCheck(ctx, selector)
	}))
	var coreVersion string
	var coreAll bool
	coreUpdate := r.leaf("update [component]", "Update a selected core or all installed cores", cobra.MaximumNArgs(1), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		selector := ""
		if len(args) > 0 {
			selector = args[0]
		}
		return r.App.CoreUpdate(ctx, selector, coreVersion, coreAll)
	})
	coreUpdate.Flags().BoolVar(&coreAll, "all", false, "Update all installed cores")
	coreUpdate.Flags().StringVar(&coreVersion, "version", "", "Select an exact version")
	cores.AddCommand(coreUpdate)
	root.AddCommand(cores)
	var updateCheck bool
	var selfVersion string
	selfUpdate := r.leaf("update", "Check or update go-proxy", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.SelfUpdate(ctx, r.Version, selfVersion, updateCheck)
	})
	selfUpdate.Flags().BoolVar(&updateCheck, "check", false, "Only check for an update")
	selfUpdate.Flags().StringVar(&selfVersion, "version", "", "Select an exact release version")
	root.AddCommand(selfUpdate)
	root.AddCommand(r.leaf("watchdog", "Run managed recovery until cancelled", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if r.JSON {
			return application.Result{}, application.Invalid("--json is not supported for watchdog")
		}
		return r.App.Watchdog(ctx)
	}))
	var lines, maxBytes int
	var follow bool
	logCommand := r.leaf("log [service]", "Read bounded logs or explicitly follow", cobra.MaximumNArgs(1), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if follow && r.JSON {
			return application.Result{}, application.Invalid("--json and --follow are mutually exclusive")
		}
		selector := ""
		if len(args) > 0 {
			selector = args[0]
		}
		return r.App.Log(ctx, selector, lines, maxBytes, follow, r.Out)
	})
	logCommand.Flags().IntVar(&lines, "lines", 50, "Maximum recent lines")
	logCommand.Flags().IntVar(&maxBytes, "max-bytes", 1<<20, "Maximum finite log output bytes")
	logCommand.Flags().BoolVar(&follow, "follow", false, "Stream until cancelled")
	root.AddCommand(logCommand)
	var preview bool
	uninstall := r.leaf("uninstall", "Preview or remove owned go-proxy resources", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if preview && r.Yes {
			return application.Result{}, application.Invalid("--preview and --yes are mutually exclusive")
		}
		if !preview {
			if err := r.confirm(); err != nil {
				return application.Result{}, err
			}
		}
		return r.App.Uninstall(ctx, preview)
	})
	uninstall.Flags().BoolVar(&preview, "preview", false, "List removal scope without changing state")
	root.AddCommand(uninstall)
}
