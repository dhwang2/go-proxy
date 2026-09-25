package cli

import (
	"bytes"

	"github.com/spf13/cobra"

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

// presetIndexes offers what rule add --rules takes, the menu indexes, each described
// by its preset's label so the shell shows "1  -- OpenAI/ChatGPT".
func presetIndexes() []string {
	menu := routing.PresetMenu()
	indexes := make([]string, 0, len(menu))
	for _, choice := range menu {
		indexes = append(indexes, choice.Symbol+"\t"+choice.Preset.Label)
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

	// Flags whose value is free-form: offer nothing rather than file names.
	freeForm := []string{
		"user", "domain", "email", "sni", "shadow-tls-sni", "port", "shadow-tls-port",
		"target", "node", "host", "parameter", "dns", "rules", "version", "lines", "max-bytes", "timeout",
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
		for _, name := range freeForm {
			if !claimed[name] && cmd.Flags().Lookup(name) != nil {
				_ = cmd.RegisterFlagCompletionFunc(name, noFiles)
			}
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

// bashDescriptionFormat replaces cobra's description formatting in the bash
// script, which pads every name to the longest and lets bash fit the
// candidates side by side. Here each is "name (description)", the description
// in lowercase, padded to the terminal width so bash lists one per line. A
// single candidate never reaches this function: cobra strips its description
// and inserts the name alone.
const bashDescriptionFormat = `
# gproxy: one candidate per line, "name (description)".
__gproxy_format_comp_descriptions()
{
    local tab=$'\t' comp desc ci
    local width=$(( ${COLUMNS:-80} - 1 ))
    for ci in ${!COMPREPLY[*]}; do
        comp=${COMPREPLY[ci]}
        [[ "$comp" == *$tab* ]] || continue
        desc=${comp#*$tab}
        comp="${comp%%$tab*} (${desc,,})"
        if (( ${#comp} > width )); then
            comp="${comp:0:width-1}…"
        fi
        printf -v comp '%-*s' "$width" "$comp"
        COMPREPLY[ci]=$comp
    done
}
`

// useBashDescriptionFormat makes `completion bash` emit cobra's script with
// bashDescriptionFormat appended; bash keeps the later definition.
func useBashDescriptionFormat(root *cobra.Command) {
	for _, command := range root.Commands() {
		if command.Name() != "completion" {
			continue
		}
		for _, shell := range command.Commands() {
			if shell.Name() != "bash" {
				continue
			}
			shell.RunE = func(cmd *cobra.Command, _ []string) error {
				noDescriptions, _ := cmd.Flags().GetBool("no-descriptions")
				var script bytes.Buffer
				if err := cmd.Root().GenBashCompletionV2(&script, !noDescriptions); err != nil {
					return err
				}
				if !noDescriptions {
					script.WriteString(bashDescriptionFormat)
				}
				_, err := cmd.OutOrStdout().Write(script.Bytes())
				return err
			}
		}
	}
}
