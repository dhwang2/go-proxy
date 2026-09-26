package cli

import (
	"context"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerSystem(r *Runner, root *cobra.Command) {
	// init and watchdog are machine entry points, not things a reader chooses.
	// install.sh runs init as its last step, and proxy-watchdog.service runs
	// watchdog as its ExecStart. Both stay reachable and documented; hiding them
	// keeps the help list to the commands a person actually picks from.
	initialize := r.leaf("init", "Initialize runtime files and dependencies; the installer runs this for you", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.Init(ctx)
	})
	initialize.Hidden = true
	root.AddCommand(initialize)
	server := &cobra.Command{Use: "server", Short: "Inspect and control managed services"}
	server.AddCommand(r.leaf("status", "Inspect managed service state", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.ServiceStatus(ctx)
	}))
	for _, action := range []string{"start", "stop", "restart"} {
		action := action
		var all bool
		selectArgs := func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return application.Invalid("select one service or --all")
			}
			if len(args) == 0 && !all {
				return guidance("gproxy server "+action+" requires a service or --all",
					[]string{"gproxy server " + action + " <" + strings.Join(application.ManagedServiceNames(), "|") + ">",
						"gproxy server " + action + " --all"},
					map[string]any{"services": application.ManagedServiceNames()})
			}
			return nil
		}
		cmd := r.leaf(action+" [service]", "Change managed service state", selectArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
			selector := ""
			if len(args) > 0 {
				selector = args[0]
			}
			return r.App.ServiceAction(ctx, action, selector, all)
		})
		cmd.Flags().BoolVar(&all, "all", false, "Select all installed managed services")
		server.AddCommand(cmd)
	}
	root.AddCommand(server)
	certificates := &cobra.Command{Use: "cert", Short: "Manage TLS certificates"}
	certificates.AddCommand(r.leaf("status", "Inspect the configured certificate", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.CertificateStatus(ctx)
	}))
	var domain, email string
	// Without --domain, ensure works on the configured domain; with none
	// configured either, the answer is the command that names one.
	ensureArgs := func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return err
		}
		if domain == "" && r.App.CertificateDomain() == "" {
			return guidance("gproxy cert ensure requires --domain: no certificate domain is configured",
				[]string{"gproxy cert ensure --domain <domain> [--email <address>]"},
				map[string]any{"missing": []string{"--domain"}})
		}
		return nil
	}
	ensure := r.leaf("ensure", "Issue a certificate for --domain, or check the configured one", ensureArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.CertificateEnsure(ctx, domain, email)
	})
	ensure.Flags().StringVar(&domain, "domain", "", "Certificate domain; the configured domain when omitted")
	ensure.Flags().StringVar(&email, "email", "", "ACME contact email")
	certificates.AddCommand(ensure)
	portArgs := func(cmd *cobra.Command, args []string) error {
		if err := atMostOne("port")(cmd, args); err != nil {
			return err
		}
		if r.App.CertificateDomain() == "" {
			return guidance("gproxy cert port needs a certificate: no certificate domain is configured",
				[]string{"gproxy cert ensure --domain <domain> [--email <address>]"}, nil)
		}
		return nil
	}
	certificates.AddCommand(r.leaf("port [port]", "Show or move the port caddy serves its site on", portArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		value := ""
		if len(args) > 0 {
			value = args[0]
		}
		return r.App.CertificatePort(ctx, value)
	}))
	root.AddCommand(certificates)
	cores := &cobra.Command{Use: "core", Short: "Inspect and update proxy cores"}
	cores.AddCommand(r.leaf("version", "Inspect installed core versions", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.CoreVersions(ctx)
	}))
	cores.AddCommand(r.leaf("check [component]", "Check available core updates", atMostOne("component"), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		selector := ""
		if len(args) > 0 {
			selector = args[0]
		}
		return r.App.CoreCheck(ctx, selector)
	}))
	var coreVersion string
	var coreAll bool
	coreUpdate := r.leaf("update [component]", "Update a selected core or all installed cores", atMostOne("component"), func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
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
	watchdog := r.leaf("watchdog", "Service entry point for proxy-watchdog; runs until cancelled", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if r.JSON {
			return application.Result{}, application.Invalid("--json is not supported for watchdog")
		}
		return r.App.Watchdog(ctx)
	})
	watchdog.Hidden = true
	watchdog.Long = "Run the managed recovery loop until cancelled.\n\n" +
		"This is the ExecStart of proxy-watchdog.service, not a command to run by hand:\n" +
		"invoked directly it occupies the terminal until interrupted. To see what it has\n" +
		"done, read its log with `gproxy log proxy-watchdog`; to control it, use\n" +
		"`gproxy server start|stop|restart proxy-watchdog`."
	root.AddCommand(watchdog)
	var lines, maxBytes int
	var follow bool
	logArgs := func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return nil
		}
		if len(args) > 1 {
			return application.Invalid("select one service")
		}
		return logGuidance()
	}
	logCommand := r.leaf("log <service>", "Read bounded logs or explicitly follow", logArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if follow && r.JSON {
			return application.Result{}, application.Invalid("--json and --follow are mutually exclusive")
		}
		// A followed log is written straight through rather than returned, so
		// the colouring and control-character stripping the rendered path does
		// has to wrap the writer instead.
		out := io.Writer(r.Out)
		if follow {
			writer := &logWriter{out: out, p: palette{on: colorEnabled(r.Out, r.NoColor)}}
			defer writer.Close()
			out = writer
		}
		return r.App.Log(ctx, args[0], lines, maxBytes, follow, out)
	})
	logCommand.Flags().IntVar(&lines, "lines", 50, "Maximum recent lines")
	logCommand.Flags().IntVar(&maxBytes, "max-bytes", 1<<20, "Maximum finite log output bytes")
	logCommand.Flags().BoolVar(&follow, "follow", false, "Stream until cancelled")
	root.AddCommand(logCommand)
	var preview bool
	// Checked as arguments so neither refusal follows a progress line: uninstall
	// is the one command where --confirm is conditional, because --preview
	// changes nothing and needs none.
	uninstallArgs := func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return err
		}
		if preview && r.Yes {
			return application.Invalid("--preview and --confirm are mutually exclusive")
		}
		if preview {
			return nil
		}
		// Neither flag: the two ways to run it, rather than an error naming
		// only the one that destroys.
		if !r.Yes {
			return guidance("gproxy uninstall requires --preview or --confirm",
				[]string{"gproxy uninstall --preview", "gproxy uninstall --confirm"},
				map[string]any{"missing": []string{"--preview", "--confirm"}})
		}
		return nil
	}
	uninstall := r.leaf("uninstall", "Preview or remove owned go-proxy resources", uninstallArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		return r.App.Uninstall(ctx, preview)
	})
	uninstall.Flags().BoolVar(&preview, "preview", false, "List removal scope without changing state")
	root.AddCommand(uninstall)
}
