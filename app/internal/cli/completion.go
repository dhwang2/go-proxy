package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"go-proxy/internal/application"
	"go-proxy/internal/core"
	"go-proxy/internal/routing"
)

// Shell completion is deliberately limited to values that are compile-time
// constants. Completing a user name, node tag or chain tag would have to load
// the store, which takes the state lock and requires root; a completion script
// runs as the invoking user, so it would contend with a running mutation and
// still return nothing. Those values are surfaced instead by the guidance in
// catalogue.go, which runs inside the real command.
//
// Everything here is answered from the in-memory command tree, so a completion
// request costs what `--help` costs.

// presetIndexes offers what rule add --rules takes: the menu indexes.
func presetIndexes() []string {
	menu := routing.PresetMenu()
	indexes := make([]string, 0, len(menu))
	for _, choice := range menu {
		indexes = append(indexes, choice.Symbol)
	}
	return indexes
}

func coreComponentNames() []string {
	components := core.AllComponents()
	names := make([]string, 0, len(components))
	for _, component := range components {
		names = append(names, string(component))
	}
	return names
}

func fixed(values []string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}

// noFiles answers a free-form flag such as --user or --domain. Without it the
// shell falls back to listing file names, which is never a valid value for
// these and actively misleads.
func noFiles(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// registerCompletions walks the built tree once and attaches candidates by
// command path. Keeping it in one place means the completion surface can be
// read, and tested, as a whole rather than hunted across seven register
// functions.
func registerCompletions(root *cobra.Command) {
	serviceNames := application.ManagedServiceNames()

	positional := map[string][]string{
		"gproxy protocol add": catalogueNames(),
		"gproxy config view":  application.ConfigKinds(),
		"gproxy core check":   coreComponentNames(),
		"gproxy core update":  coreComponentNames(),
		"gproxy log":          serviceNames,

		"gproxy route direct set": application.DirectStrategies(),
	}
	for _, action := range []string{"start", "stop", "restart"} {
		positional["gproxy server "+action] = serviceNames
	}

	flags := map[string]map[string][]string{
		"gproxy protocol add":      {"congestion": {"bbr", "cubic"}},
		"gproxy route rule add":    {"rules": presetIndexes(), "out": {"direct"}},
		"gproxy route rule modify": {"out": {"direct"}},
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		path := cmd.CommandPath()
		if values, ok := positional[path]; ok && len(cmd.ValidArgs) == 0 {
			// ValidArgs drives completion without enforcing membership, so the
			// existing guidance validators keep producing their own errors.
			cmd.ValidArgs = values
		}
		claimed := map[string]bool{}
		for flag, values := range flags[path] {
			if cmd.Flags().Lookup(flag) != nil {
				_ = cmd.RegisterFlagCompletionFunc(flag, fixed(values))
				claimed[flag] = true
			}
		}
		// Nothing gproxy takes is a file, so where there is no candidate to
		// offer, Tab offers nothing: without this the shell falls back to
		// listing the files in the current directory, as after `gproxy status`.
		if cmd.ValidArgsFunction == nil && len(cmd.ValidArgs) == 0 {
			cmd.ValidArgsFunction = noFiles
		}
		for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.PersistentFlags()} {
			set.VisitAll(func(flag *pflag.Flag) {
				if !claimed[flag.Name] && flag.Value.Type() != "bool" {
					// A flag already registered, or one inherited, keeps its
					// own completion; the error only says so.
					_ = cmd.RegisterFlagCompletionFunc(flag.Name, noFiles)
				}
			})
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}
