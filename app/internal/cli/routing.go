package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go-proxy/internal/application"
	"go-proxy/internal/routing"
)

// parseChainParameter splits --parameter host:port[:username:password]. An
// IPv6 host is written in brackets, as in a URL. Everything after the third
// colon is the password, so a password may itself contain colons; a username
// cannot. The value is never echoed in an error, because it carries the
// password.
//
// The credentials travel on the command line by the operator's choice, which
// puts them in the shell history and the process listing; the documented
// secret-input rule carries this flag as its exception.
func parseChainParameter(value string) (host string, port int, username, password string, err error) {
	format := application.Invalid("--parameter must be host:port or host:port:username:password; write an ipv6 host in brackets")
	rest := strings.TrimSpace(value)
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 || !strings.HasPrefix(rest[end+1:], ":") {
			return "", 0, "", "", format
		}
		host, rest = rest[1:end], rest[end+2:]
	} else {
		var found bool
		if host, rest, found = strings.Cut(rest, ":"); !found {
			return "", 0, "", "", format
		}
	}
	fields := strings.SplitN(rest, ":", 3)
	port, err = strconv.Atoi(fields[0])
	if host == "" || err != nil {
		return "", 0, "", "", format
	}
	switch len(fields) {
	case 2:
		return "", 0, "", "", application.Invalid("--parameter takes a username and a password together")
	case 3:
		username, password = fields[1], fields[2]
		if username == "" || password == "" {
			return "", 0, "", "", application.Invalid("--parameter takes a username and a password together")
		}
	}
	return host, port, username, password, nil
}

func registerRouting(r *Runner, root *cobra.Command) {
	group := &cobra.Command{Use: "route", Short: "Inspect and manage routing rules, chains and direct egress"}
	root.AddCommand(group)
	rule := &cobra.Command{Use: "rule", Short: "Manage per-user routing rules"}
	group.AddCommand(rule)

	var listUser string
	ruleList := r.leaf("list", "List rule indexes and matchers", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingList(ctx, listUser)
	})
	ruleList.Flags().StringVar(&listUser, "user", "", "Limit to one user")
	rule.AddCommand(ruleList)

	var addUser, addOut string
	var addPresets []string
	addArgs := func(cmd *cobra.Command, _ []string) error {
		missing := []string{}
		if addUser == "" {
			missing = append(missing, "--user")
		}
		if len(addPresets) == 0 {
			missing = append(missing, "--rules")
		}
		if addOut == "" {
			missing = append(missing, "--out")
		}
		if len(missing) > 0 {
			return presetGuidance(missing)
		}
		// Indexes are resolved here, with the other arguments, so an unknown
		// one is refused before the operation starts. A preset named twice is
		// added once.
		resolved := make([]string, 0, len(addPresets))
		for _, selector := range addPresets {
			name, ok := routing.ResolvePresetSelector(selector)
			if !ok {
				return application.Invalid("unknown preset " + strings.TrimSpace(selector) + "; run gproxy route rule add to list them")
			}
			if !slices.Contains(resolved, name) {
				resolved = append(resolved, name)
			}
		}
		addPresets = resolved
		return nil
	}
	ruleAdd := r.leaf("add", "Add preset rules for one user", addArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingSet(ctx, addUser, addPresets, addOut)
	})
	ruleAdd.Flags().StringVar(&addUser, "user", "", "User the rules belong to")
	ruleAdd.Flags().StringSliceVar(&addPresets, "rules", nil, "Comma-separated preset indexes from the menu (1,3,a)")
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
			if len(missing) > 0 {
				return ruleSelectGuidance(action, missing)
			}
			// Checked with the other arguments, so a value no rule can have is
			// refused before the operation starts.
			for _, selector := range indexes {
				if !routing.ValidRuleSelector(selector) {
					return application.Invalid("unknown rule " + strings.TrimSpace(selector) + "; run gproxy route rule list")
				}
			}
			return nil
		}
		short := "Remove selected rules, or every rule with --all"
		if action == "modify" {
			short = "Point selected rules at a different outbound"
		}
		checked := cobra.PositionalArgs(selectArgs)
		if action == "remove" {
			checked = r.confirming(selectArgs)
		}
		cmd := r.leaf(action, short, checked, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
			if action == "remove" && all {
				return r.App.RoutingClear(ctx, user, all)
			}
			return r.App.RoutingRules(ctx, user, indexes, out, action == "remove")
		})
		cmd.Flags().StringVar(&user, "user", "", "User the rules belong to")
		cmd.Flags().StringSliceVar(&indexes, "rules", nil, "Comma-separated rule numbers from route rule list (1,3,a)")
		if action == "modify" {
			cmd.Flags().StringVar(&out, "out", "", "Outbound: direct or a chain tag")
		} else {
			cmd.Flags().BoolVar(&all, "all", false, "Remove every rule for --user, or for all users when --user is omitted")
		}
		rule.AddCommand(cmd)
	}
	// direct is a group with list and set rather than one command carrying a
	// --strategy flag, so reading and writing the strategy are named the way
	// every other object in the tree names them. list rather than show even for
	// a single value: one verb per operation across the whole tree is worth more
	// than a name that fits one command.
	direct := &cobra.Command{Use: "direct", Short: "Inspect or set the direct DNS/IP strategy"}
	direct.AddCommand(r.leaf("list", "List the direct DNS/IP strategy", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingDirect(ctx, "", false)
	}))
	directSet := r.leaf("set <strategy>", "Set the direct DNS/IP strategy", func(cmd *cobra.Command, args []string) error {
		// Checked here as well as in the application, so an unusable value is
		// refused before the mutation progress line rather than after it. The
		// application keeps its own check: it does not trust this one.
		if len(args) == 1 && slices.Contains(application.DirectStrategies(), args[0]) {
			return nil
		}
		return strategyGuidance(args)
	}, func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingDirect(ctx, args[0], true)
	})
	direct.AddCommand(directSet)
	group.AddCommand(direct)
	// final is where connections no rule claims leave. direct is the default;
	// a chain sends the whole server through it, shell-proxy's global mode.
	final := &cobra.Command{Use: "final", Short: "Inspect or set where unmatched traffic leaves"}
	final.AddCommand(r.leaf("list", "List where unmatched traffic leaves", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingFinal(ctx, "", false)
	}))
	final.AddCommand(r.leaf("set <direct|chain tag>", "Send unmatched traffic direct or through a chain", func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return nil
		}
		return finalGuidance(args)
	}, func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingFinal(ctx, args[0], true)
	}))
	group.AddCommand(final)
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
			[]string{"gproxy route test --user <name> --domain <domain|ip>",
				"gproxy route test --user alice --domain github.com"},
			map[string]any{"missing": missing})
	}
	test := r.leaf("test", "Show which rule takes a domain or IP, without sending traffic", testArgs, func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingTest(ctx, testUser, testDomain)
	})
	test.Flags().StringVar(&testUser, "user", "", "User whose rules are evaluated")
	test.Flags().StringVar(&testDomain, "domain", "", "Domain or IP address to evaluate")
	group.AddCommand(test)
	chain := &cobra.Command{Use: "chain", Short: "Manage SOCKS5 chain outbounds", Args: cobra.NoArgs}
	chain.AddCommand(r.leaf("list", "List chain outbounds with credentials redacted", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingChains(ctx)
	}))
	// Guidance replaces cobra's bare "accepts 1 arg(s), received 0" and the
	// required-flag error, neither of which says what the argument is or that
	// credentials come from a file.
	chainArgs := func(cmd *cobra.Command, args []string) error {
		missing := []string{}
		if len(args) == 0 {
			missing = append(missing, "<tag>")
		}
		if len(args) > 1 {
			return application.Invalid("select one chain tag")
		}
		value, _ := cmd.Flags().GetString("parameter")
		if strings.TrimSpace(value) == "" {
			missing = append(missing, "--parameter")
		}
		if len(missing) > 0 {
			return chainGuidance(missing)
		}
		// Checked here, with the other arguments, so a malformed value is
		// refused before the operation starts rather than after its first
		// progress line.
		_, _, _, _, err := parseChainParameter(value)
		return err
	}
	add := r.leaf("add <tag>", "Add a named SOCKS5 chain outbound", chainArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		value, _ := cmd.Flags().GetString("parameter")
		host, port, username, password, err := parseChainParameter(value)
		if err != nil {
			return application.Result{}, err
		}
		resolver, _ := cmd.Flags().GetString("dns")
		return r.App.RoutingChainAdd(ctx, args[0], host, port, username, password, strings.TrimSpace(resolver))
	})
	add.Flags().String("parameter", "", "SOCKS5 server as host:port or host:port:username:password")
	add.Flags().String("dns", "", "Resolver for this chain, reached through it (default: https://dns.google/dns-query)")
	chain.AddCommand(add)
	modifyArgs := func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return application.Invalid("select one chain tag")
		}
		changed := cmd.Flags().Changed("parameter") || cmd.Flags().Changed("dns")
		if len(args) == 0 || !changed {
			return chainModifyGuidance(len(args) == 0)
		}
		if cmd.Flags().Changed("parameter") {
			value, _ := cmd.Flags().GetString("parameter")
			if _, _, _, _, err := parseChainParameter(value); err != nil {
				return err
			}
		}
		return nil
	}
	// --parameter replaces the whole endpoint: host, port and credentials
	// together, so host:port alone makes the chain unauthenticated.
	modify := r.leaf("modify <tag>", "Change where an existing chain points", modifyArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		resolver, _ := cmd.Flags().GetString("dns")
		change := application.ChainChange{Resolver: strings.TrimSpace(resolver)}
		if cmd.Flags().Changed("parameter") {
			value, _ := cmd.Flags().GetString("parameter")
			host, port, username, password, err := parseChainParameter(value)
			if err != nil {
				return application.Result{}, err
			}
			change.Host, change.Port = host, port
			change.Username, change.Password, change.SetCredentials = username, password, true
		}
		return r.App.RoutingChainModify(ctx, args[0], change)
	})
	modify.Flags().String("parameter", "", "SOCKS5 server as host:port or host:port:username:password")
	modify.Flags().String("dns", "", "Resolver for this chain, reached through it")
	chain.AddCommand(modify)
	chainRemoveArgs := func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return application.Invalid("select one chain tag")
		}
		if len(args) == 0 {
			return chainRemoveGuidance()
		}
		return nil
	}
	chain.AddCommand(r.leaf("remove <tag>", "Remove an unused chain outbound", r.confirming(chainRemoveArgs), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		result, err := r.App.RoutingChainRemove(ctx, args[0])
		return result, chainInUse(err)
	}))
	group.AddCommand(chain)
}

// chainInUse turns remove's refusal into guidance: the rules that still send
// traffic through the chain, listed as route rule list prints them, then for
// each user the two commands that take them off it, with the selectors filled
// in. A chain held only by the route final keeps its error, which already
// names the command.
func chainInUse(err error) error {
	var refusal *application.Error
	if !errors.As(err, &refusal) || refusal.Code != "conflict" {
		return err
	}
	fields, _ := refusal.Data.(map[string]any)
	rules, _ := fields["rules"].([]application.RouteEntry)
	final, _ := fields["route_final"].(bool)
	if len(rules) == 0 {
		return err
	}
	rules = slices.Clone(rules)
	sort.SliceStable(rules, func(i, j int) bool {
		return routing.PresetRank(rules[i].Preset) < routing.PresetRank(rules[j].Preset)
	})
	users := []string{}
	selectors := map[string][]string{}
	for _, entry := range rules {
		if _, seen := selectors[entry.User]; !seen {
			users = append(users, entry.User)
		}
		selectors[entry.User] = append(selectors[entry.User], entry.Selector)
	}
	sort.Strings(users)
	hint := make([]string, 0, 2*len(users)+1)
	for _, user := range users {
		selected := strings.Join(selectors[user], ",")
		hint = append(hint,
			"gproxy route rule modify --user "+user+" --rules "+selected+" --out direct",
			"gproxy route rule remove --user "+user+" --rules "+selected+" --confirm")
	}
	if final {
		hint = append(hint, "gproxy route final set direct (the chain is also the route final)")
	}
	guided := *refusal
	guided.Hint = hint
	return &listedError{guidance: &guided, list: func(w io.Writer, p palette) {
		writeRuleList(w, p, rules, nil, nil)
		fmt.Fprintln(w)
	}}
}
