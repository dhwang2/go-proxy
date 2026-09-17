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
	"go-proxy/internal/routing"
)

func registerRouting(r *Runner, root *cobra.Command) {
	group := r.leaf("routing", "List and manage user routing", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingList(ctx, "")
	})
	root.AddCommand(group)
	group.AddCommand(r.leaf("list [user]", "List rule indexes and matchers", cobra.MaximumNArgs(1), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return r.App.RoutingList(ctx, name)
	}))
	group.AddCommand(r.leaf("presets", "List built-in routing presets", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		presets := []routing.Preset{}
		for _, preset := range routing.BuiltinPresets() {
			if preset.Name != "custom" {
				presets = append(presets, preset)
			}
		}
		return application.Result{Data: map[string]any{"presets": presets}}, nil
	}))
	set := r.leaf("set <user>", "Add or update one or more preset rules", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		presets, _ := cmd.Flags().GetStringSlice("preset")
		outbound, _ := cmd.Flags().GetString("outbound")
		return r.App.RoutingSet(ctx, args[0], presets, outbound)
	})
	set.Flags().StringSlice("preset", nil, "Comma-separated preset names")
	set.Flags().String("outbound", "", "Outbound tag or direct")
	_ = set.MarkFlagRequired("preset")
	_ = set.MarkFlagRequired("outbound")
	group.AddCommand(set)
	for _, action := range []string{"remove", "modify"} {
		cmd := r.leaf(action+" <user>", "Change selected user rules by current 1-based indexes", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
			raw, _ := cmd.Flags().GetStringSlice("rules")
			indexes := make([]int, 0, len(raw))
			for _, value := range raw {
				index, err := strconv.Atoi(value)
				if err != nil || index < 1 {
					return application.Result{}, application.Invalid("rule indexes must be positive integers")
				}
				indexes = append(indexes, index)
			}
			outbound := ""
			if action == "remove" {
				if err := r.confirm(); err != nil {
					return application.Result{}, err
				}
			} else {
				outbound, _ = cmd.Flags().GetString("outbound")
			}
			return r.App.RoutingRules(ctx, args[0], indexes, outbound, action == "remove")
		})
		cmd.Flags().StringSlice("rules", nil, "Comma-separated rule indexes from routing list")
		_ = cmd.MarkFlagRequired("rules")
		if action == "modify" {
			cmd.Flags().String("outbound", "", "Outbound tag or direct")
			_ = cmd.MarkFlagRequired("outbound")
		}
		group.AddCommand(cmd)
	}
	clear := r.leaf("clear [user]", "Clear routing for one user or all users", cobra.MaximumNArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (application.Result, error) {
		all, _ := cmd.Flags().GetBool("all")
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		if all == (name != "") {
			return application.Result{}, application.Invalid("select one user or --all")
		}
		if err := r.confirm(); err != nil {
			return application.Result{}, err
		}
		return r.App.RoutingClear(ctx, name, all)
	})
	clear.Flags().Bool("all", false, "Clear all user routing rules")
	group.AddCommand(clear)
	direct := r.leaf("direct", "Inspect or set the direct DNS/IP strategy", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, _ []string) (application.Result, error) {
		strategy, _ := cmd.Flags().GetString("strategy")
		return r.App.RoutingDirect(ctx, strategy, cmd.Flags().Changed("strategy"))
	})
	direct.Flags().String("strategy", "", "ipv4_only, ipv6_only, prefer_ipv4, prefer_ipv6 or asis")
	group.AddCommand(direct)
	group.AddCommand(r.leaf("sync-dns", "Recompile routes and DNS with the saved strategy", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
		return r.App.RoutingSyncDNS(ctx)
	}))
	group.AddCommand(r.leaf("test <user> <domain>", "Explain local rule matching without sending proxy traffic", cobra.ExactArgs(2), func(ctx context.Context, _ *cobra.Command, args []string) (application.Result, error) {
		return r.App.RoutingTest(ctx, args[0], args[1])
	}))
	chain := &cobra.Command{Use: "chain", Short: "Manage SOCKS5 chain outbounds", Args: cobra.NoArgs}
	chain.AddCommand(r.leaf("list", "List chain outbounds with credentials redacted", cobra.NoArgs, func(ctx context.Context, _ *cobra.Command, _ []string) (application.Result, error) {
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
