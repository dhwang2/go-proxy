package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
)

func registerRouting(r *Runner, root *cobra.Command) {
	group := &cobra.Command{Use: "route", Short: "Inspect and manage routing rules, chains and direct egress"}
	root.AddCommand(group)
	group.AddCommand(r.leaf("show", "Show rules, chain outbounds and the direct strategy", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingOverview(ctx)
	}))
	rule := &cobra.Command{Use: "rule", Short: "Manage per-user routing rules"}
	group.AddCommand(rule)

	var showUser string
	ruleShow := r.leaf("show", "List rule indexes and matchers", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingList(ctx, showUser)
	})
	ruleShow.Flags().StringVar(&showUser, "user", "", "Limit to one user")
	rule.AddCommand(ruleShow)

	var addUser, addOut string
	var addPresets []string
	addArgs := func(cmd *cobra.Command, _ []string) error {
		missing := []string{}
		if addUser == "" {
			missing = append(missing, "--user")
		}
		if len(addPresets) == 0 {
			missing = append(missing, "--preset")
		}
		if addOut == "" {
			missing = append(missing, "--out")
		}
		if len(missing) > 0 {
			return presetGuidance(missing)
		}
		return nil
	}
	ruleAdd := r.leaf("add", "Add preset rules for one user", addArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingSet(ctx, addUser, addPresets, addOut)
	})
	ruleAdd.Flags().StringVar(&addUser, "user", "", "User the rules belong to")
	ruleAdd.Flags().StringSliceVar(&addPresets, "preset", nil, "Comma-separated preset names")
	ruleAdd.Flags().StringVar(&addOut, "out", "", "Outbound: direct or a chain tag")
	rule.AddCommand(ruleAdd)
	for _, action := range []string{"remove", "modify"} {
		action := action
		var user, out string
		var indexes []string
		var all bool
		selectArgs := func(cmd *cobra.Command, _ []string) error {
			if action == "remove" && all && len(indexes) > 0 {
				return application.Invalid("--rules and --all are mutually exclusive")
			}
			missing := []string{}
			if user == "" && !(action == "remove" && all) {
				missing = append(missing, "--user")
			}
			if action == "modify" && out == "" {
				missing = append(missing, "--out")
			}
			if len(indexes) == 0 && !(action == "remove" && all) {
				missing = append(missing, "--rules")
			}
			if len(missing) == 0 {
				return nil
			}
			example := "  run: gproxy route rule " + action + " --user alice --rules 1,2"
			if action == "modify" {
				example += " --out direct"
			}
			return guidance(joinList(missing)+" required for gproxy route rule "+action,
				[]string{"  gproxy route rule show   lists the current indexes", example},
				map[string]any{"missing": missing})
		}
		short := "Remove selected rules, or every rule with --all"
		if action == "modify" {
			short = "Point selected rules at a different outbound"
		}
		cmd := r.leaf(action, short, selectArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
			if action == "remove" {
				if err := r.confirm(); err != nil {
					return application.Result{}, err
				}
				if all {
					return r.App.RoutingClear(ctx, user, user == "")
				}
			}
			parsed := make([]int, 0, len(indexes))
			for _, value := range indexes {
				index, err := strconv.Atoi(value)
				if err != nil || index < 1 {
					return application.Result{}, application.Invalid("rule indexes must be positive integers")
				}
				parsed = append(parsed, index)
			}
			return r.App.RoutingRules(ctx, user, parsed, out, action == "remove")
		})
		cmd.Flags().StringVar(&user, "user", "", "User the rules belong to")
		cmd.Flags().StringSliceVar(&indexes, "rules", nil, "Comma-separated rule indexes from route rule show")
		if action == "modify" {
			cmd.Flags().StringVar(&out, "out", "", "Outbound: direct or a chain tag")
		} else {
			cmd.Flags().BoolVar(&all, "all", false, "Remove every rule for --user, or for all users when --user is omitted")
		}
		rule.AddCommand(cmd)
	}
	direct := r.leaf("direct", "Inspect or set the direct DNS/IP strategy", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, _ []string) (application.Result, error) {
		strategy, _ := cmd.Flags().GetString("strategy")
		return r.App.RoutingDirect(ctx, strategy, cmd.Flags().Changed("strategy"))
	})
	direct.Flags().String("strategy", "", "ipv4_only, ipv6_only, prefer_ipv4, prefer_ipv6 or asis")
	group.AddCommand(direct)
	group.AddCommand(r.leaf("sync-dns", "Recompile routes and DNS with the saved strategy", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingSyncDNS(ctx)
	}))
	var testUser, testDomain string
	testArgs := func(cmd *cobra.Command, _ []string) error {
		missing := []string{}
		if testUser == "" {
			missing = append(missing, "--user")
		}
		if testDomain == "" {
			missing = append(missing, "--domain")
		}
		if len(missing) == 0 {
			return nil
		}
		return guidance(joinList(missing)+" required for gproxy route test",
			[]string{"  run: gproxy route test --user alice --domain github.com",
				"  reports which rule matches; it sends no proxy traffic"},
			map[string]any{"missing": missing})
	}
	test := r.leaf("test", "Explain local rule matching without sending proxy traffic", testArgs, func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingTest(ctx, testUser, testDomain)
	})
	test.Flags().StringVar(&testUser, "user", "", "User whose rules are evaluated")
	test.Flags().StringVar(&testDomain, "domain", "", "Domain to evaluate")
	group.AddCommand(test)
	chain := &cobra.Command{Use: "chain", Short: "Manage SOCKS5 chain outbounds", Args: cobra.NoArgs}
	chain.AddCommand(r.leaf("show", "List chain outbounds with credentials redacted", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingChains(ctx)
	}))
	add := r.leaf("add <tag>", "Add a named SOCKS5 chain outbound", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		path, _ := cmd.Flags().GetString("credentials-file")
		var credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if cmd.Flags().Changed("credentials-file") {
			if path == "" {
				return application.Result{}, application.Invalid("credentials-file cannot be empty")
			}
			var reader io.Reader = r.In
			if path != "-" {
				file, err := os.Open(path)
				if err != nil {
					return application.Result{}, application.Invalid("cannot open credentials file")
				}
				defer file.Close()
				reader = file
			}
			type inputResult struct {
				data []byte
				err  error
			}
			input := make(chan inputResult, 1)
			go func() { data, err := io.ReadAll(io.LimitReader(reader, 65537)); input <- inputResult{data, err} }()
			var raw []byte
			select {
			case result := <-input:
				if result.err != nil || len(result.data) > 65536 {
					return application.Result{}, application.Invalid("credentials file must be readable and at most 64 kib")
				}
				raw = result.data
			case <-ctx.Done():
				if closer, ok := reader.(io.Closer); ok {
					_ = closer.Close()
				}
				return application.Result{}, ctx.Err()
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&credentials) != nil || credentials.Username == "" || credentials.Password == "" {
				return application.Result{}, application.Invalid("credentials file must contain username and password")
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				return application.Result{}, application.Invalid("credentials file must contain one json object")
			}
		}
		return r.App.RoutingChainAdd(ctx, args[0], strings.TrimSpace(host), port, credentials.Username, credentials.Password)
	})
	add.Flags().String("host", "", "SOCKS5 server host")
	add.Flags().Int("port", 0, "SOCKS5 server port")
	add.Flags().String("credentials-file", "", "JSON credentials file, or - for explicitly selected stdin")
	_ = add.MarkFlagRequired("host")
	_ = add.MarkFlagRequired("port")
	chain.AddCommand(add)
	chain.AddCommand(r.leaf("remove <tag>", "Remove an unused chain outbound", cobra.ExactArgs(1), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		if err := r.confirm(); err != nil {
			return application.Result{}, err
		}
		return r.App.RoutingChainRemove(ctx, args[0])
	}))
	group.AddCommand(chain)
}
