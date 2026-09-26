package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"go-proxy/internal/application"
	"go-proxy/internal/routing"
)

// protocolEntry describes one installable protocol well enough to answer both
// "what can I install" and "how do I install this one" without the reader
// opening documentation or reading a flag list that mostly does not apply.
type protocolEntry struct {
	Name    string
	Summary string
	// Variants are the forms this protocol can take, shown after the name in
	// the quick reference. Absent means the protocol has only one form.
	Variants []string
	// Usage is the full flag signature. Required flags are bare, optional ones
	// bracketed, alternatives separated by |. This is what a reader who already
	// knows the protocols needs: not what VLESS is, but what to type. It is the
	// whole guidance, because every example that used to follow it was a subset
	// of it with the brackets resolved one way.
	Usage   string
	Options []string
}

var protocolCatalogue = []protocolEntry{
	{
		Name:     "vless",
		Usage:    "--user <name> --port <port|auto> [--domain <domain> --email <address> | --reality --sni <domain>]",
		Variants: []string{"tls", "reality"},
		Summary:  "VLESS over TLS, or Reality without a certificate",
		Options:  []string{"--port <port|auto>", "--domain <certificate domain>", "--reality", "--sni <handshake domain>"},
	},
	{
		Name:    "tuic",
		Usage:   "--user <name> --port <port|auto> --domain <domain> [--email <address>] [--congestion <bbr|cubic>]",
		Summary: "TUIC v5 over UDP, requires a certificate",
		Options: []string{"--port <port|auto>", "--domain <certificate domain>", "--congestion <bbr|cubic>"},
	},
	{
		Name:    "anytls",
		Usage:   "--user <name> --port <port|auto> --domain <domain> [--email <address>]",
		Summary: "AnyTLS, requires a certificate",
		Options: []string{"--port <port|auto>", "--domain <certificate domain>"},
	},
	{
		Name:     "snell",
		Usage:    "--user <name> --port <port|auto> [--mode <default|unshaped>] [--dns-ip-preference <default|prefer-ipv4|prefer-ipv6|ipv4-only|ipv6-only>] [--dns <resolver-ip,...>] [--egress-interface <network-interface>] [--shadow-tls [--shadow-tls-port <port|auto>] [--shadow-tls-sni <domain>]]",
		Variants: []string{"v6", "tls"},
		Summary:  "Snell v6, one node per host, optionally wrapped in ShadowTLS",
		Options:  []string{"--port <port|auto>", "--mode <default|unshaped>", "--dns-ip-preference <default|prefer-ipv4|prefer-ipv6|ipv4-only|ipv6-only>", "--dns <resolver-ip,...>", "--egress-interface <network-interface>", "--shadow-tls", "--shadow-tls-port <port|auto>", "--shadow-tls-sni <domain>"},
	},
}

// joinList renders a flag list the way a sentence needs it: "--user",
// "--user and --port", "--user, --rules and --out".
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func catalogueEntry(name string) (protocolEntry, bool) {
	for _, entry := range protocolCatalogue {
		if entry.Name == name {
			return entry, true
		}
	}
	return protocolEntry{}, false
}

func catalogueNames() []string {
	names := make([]string, 0, len(protocolCatalogue))
	for _, entry := range protocolCatalogue {
		names = append(names, entry.Name)
	}
	return names
}

func catalogueData() []map[string]string {
	entries := make([]map[string]string, 0, len(protocolCatalogue))
	for _, entry := range protocolCatalogue {
		entries = append(entries, map[string]string{
			"name": entry.Name, "summary": entry.Summary, "display": entry.display(),
		})
	}
	return entries
}

// display is the quick-reference spelling: the name, and the forms it comes in
// when there is more than one. The prose summary belongs to the guidance a
// reader sees when they do not yet know what to pick.
func (e protocolEntry) display() string {
	if len(e.Variants) == 0 {
		return e.Name
	}
	return e.Name + " (" + strings.Join(e.Variants, "/") + ")"
}

// guidance is the failure a command returns when it was named but not given
// enough to act. It is an ordinary invalid_argument, so it reaches stderr at
// exit 2 and a caller cannot read it as a result; Data carries the same choices
// in a form a machine reader can use.
func guidance(message string, hint []string, data any) error {
	return &application.Error{Code: "invalid_argument", Message: message, Hint: hint, Data: data}
}

// atMostOne refuses a second argument in this CLI's words; cobra's own
// MaximumNArgs answers "accepts at most 1 arg(s), received 2".
func atMostOne(what string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 1 {
			return application.Invalid("select one " + what)
		}
		return nil
	}
}

// listedError is guidance that shows what is in the way before the commands
// that clear it, drawn in the colours the listing has everywhere else. The
// hint lines pass through redaction, which strips escape sequences, so the
// listing cannot travel as hint text.
type listedError struct {
	guidance *application.Error
	list     func(io.Writer, palette)
}

func (e *listedError) Error() string { return e.guidance.Message }
func (e *listedError) Unwrap() error { return e.guidance }

// chooseProtocolGuidance prints a command reference rather than a description
// of each protocol: one full command per protocol, flush left, each with every
// option that protocol takes. The reader is assumed to know what VLESS is;
// what they need is the flags it takes.
func chooseProtocolGuidance() error {
	// Bare names, not display(): these lines are typed, and a name with
	// brackets is not an argument the parser accepts.
	hint := make([]string, 0, len(protocolCatalogue))
	for _, entry := range protocolCatalogue {
		hint = append(hint, "gproxy protocol add "+entry.Name+" "+entry.Usage)
	}
	return guidance("gproxy protocol add requires a protocol: "+strings.Join(catalogueNames(), ", "),
		hint, map[string]any{"protocols": catalogueData()})
}

// installGuidance answers an install that named a protocol but not enough to
// run: the one command that completes it, unindented so it can be taken off the
// screen as it stands. The examples that used to follow were each that same
// line with its brackets resolved one way, and three of them said less than the
// line above them.
//
// entry.Name, not display(): this line is meant to be typed, and
// "vless(tls/reality)" is not an argument the parser accepts.
func installGuidance(entry protocolEntry, missing []string) error {
	return guidance(joinList(missing)+" required for gproxy protocol add "+entry.Name,
		[]string{"gproxy protocol add " + entry.Name + " " + entry.Usage},
		map[string]any{"protocol": entry.Name, "missing": missing, "usage": entry.Usage, "options": entry.Options})
}

// presetGuidance replaces the separate `routing presets` command: the menu is
// only ever needed while composing a rule, which is exactly when this prints.
// It is laid out like the install guidance, flush left: one command that runs
// as written, then the menu in shell-proxy's numbering, whose indexes the
// command takes, several at once.
func presetGuidance(missing []string) error {
	menu := routing.PresetMenu()
	hint := make([]string, 0, len(menu)+1)
	hint = append(hint, "gproxy route rule add --user alice --rules 1,3,5,9,a,b --out direct")
	names := make([]string, 0, len(menu))
	indexes := make(map[string]string, len(menu))
	for _, choice := range menu {
		hint = append(hint, choice.Symbol+"."+choice.Preset.Label)
		names = append(names, choice.Preset.Name)
		indexes[choice.Symbol] = choice.Preset.Name
	}
	return guidance(joinList(annotateIndexed(missing, "--rules"))+" required for gproxy route rule add",
		hint, map[string]any{"missing": missing, "presets": names, "indexes": indexes})
}

// ruleSelectGuidance answers a modify or remove that did not say which rules:
// the numbers route rule list prints, which are the preset menu's, several at
// once.
func ruleSelectGuidance(action string, missing []string) error {
	hint := []string{"gproxy route rule list --user alice"}
	if action == "modify" {
		hint = append(hint, "gproxy route rule modify --user alice --rules 1,3,a --out res1")
	} else {
		hint = append(hint,
			"gproxy route rule remove --user alice --rules 1,3,a --confirm",
			"gproxy route rule remove --user alice --all --confirm")
	}
	return guidance(joinList(annotateIndexed(missing, "--rules"))+" required for gproxy route rule "+action,
		hint, map[string]any{"missing": missing})
}

// annotateIndexed marks the flag that takes indexes, so the error line itself
// says several can be given.
func annotateIndexed(missing []string, flag string) []string {
	out := make([]string, len(missing))
	for index, name := range missing {
		out[index] = name
		if name == flag {
			out[index] = name + " (one or more indexes, comma-separated)"
		}
	}
	return out
}

// chainGuidance answers an add that is missing its tag or endpoint the way
// installGuidance does: the one command with every optional part, unindented
// so it can be taken off the screen, then examples that run as written. The
// examples carry what prose used to explain: the bracketed IPv6 host, the
// credentials going together, and --dns.
func chainGuidance(missing []string) error {
	return guidance(joinList(missing)+" required for gproxy route chain add",
		[]string{
			"gproxy route chain add <tag> --parameter <host>:<port>[:<username>:<password>] [--dns <resolver>]",
			"gproxy route chain add res1 --parameter 198.51.100.7:1080",
			"gproxy route chain add res1 --parameter 198.51.100.7:1080:alice:secret",
			"gproxy route chain add res1 --parameter [2001:db8::1]:1080:alice:secret",
			"gproxy route chain add res1 --parameter 198.51.100.7:1080:alice:secret --dns https://dns.quad9.net/dns-query",
		},
		map[string]any{"missing": missing, "parameter_format": "host:port[:username:password]"})
}

// removeGuidance lists what can actually be removed. A reader asking to remove
// something needs the tags that exist now, which no static list can provide.
func removeGuidance(nodes []application.ProtocolNode) error {
	if len(nodes) == 0 {
		// Nothing to choose from, so there is nothing to explain. The JSON
		// envelope still carries the usual invalid_argument and an empty list.
		// Hint carries the human answer, Message the machine one: a reader wants
		// the short form, a caller parsing the envelope wants a sentence. The
		// short form is the one `protocol list` prints for the same state -- a
		// bare "empty" left the reader to guess what was empty.
		return &application.Error{Code: "invalid_argument", Hint: []string{"no nodes"},
			JSONMessage: "no protocol nodes are installed",
			Data:        map[string]any{"nodes": []string{}}}
	}
	// The row number is what this command takes, so the first column is the
	// node's name with the port stripped from it: the port is already the next
	// column, and repeating it in the name said it twice.
	names := make([]string, 0, len(nodes))
	width := 0
	for _, node := range nodes {
		name := clean(application.NodeName(node))
		names = append(names, name)
		if size := displayWidth(name); size > width {
			width = size
		}
	}
	portWidth, securityWidth := nodeColumnWidths(nodes)
	// The command first, then the nodes whose numbers it takes, as the preset
	// menu is laid out.
	hint := make([]string, 0, len(nodes)+1)
	hint = append(hint, "gproxy protocol remove <index|tag> [--user <name>] --confirm")
	tags := make([]string, 0, len(nodes))
	for index, node := range nodes {
		owners := strings.Join(nodeOwners(node), "/")
		if owners == "" {
			owners = "no users"
		}
		hint = append(hint, strings.TrimRight(fmt.Sprintf("%d.%s  %s  %s  %s",
			index+1, pad(names[index], width),
			pad(fmt.Sprintf("%d", node.Port), portWidth),
			pad(nodeSecurity(node), securityWidth), owners), " "))
		tags = append(tags, node.Tag)
	}
	return guidance("gproxy protocol remove requires a node",
		hint, map[string]any{"nodes": tags})
}

// The user commands take a name and nothing else: the full command, and for
// rename and remove one that runs as written; add is the command alone. An
// invalid name is refused with the rule it broke.
func addUserGuidance() error {
	return guidance("gproxy user add requires a name",
		[]string{"gproxy user add <name> [--all-protocols]"},
		map[string]any{"missing": []string{"<name>"}})
}

func renameUserGuidance() error {
	return guidance("gproxy user rename requires the current and the new name",
		[]string{"gproxy user rename <old> <new>", "gproxy user rename alice bob"},
		map[string]any{"missing": []string{"<old>", "<new>"}})
}

func removeUserGuidance() error {
	return guidance("gproxy user remove requires a name",
		[]string{"gproxy user remove <name> --confirm", "gproxy user remove alice --confirm"},
		map[string]any{"missing": []string{"<name>"}})
}

// configGuidance names the inspectable sources inside the command itself: a
// closed set the reader cannot guess.
func configGuidance(args []string) error {
	message := "gproxy config view requires a configuration"
	if len(args) > 1 {
		message = "gproxy config view takes one configuration"
	}
	kinds := application.ConfigKinds()
	return guidance(message,
		[]string{"gproxy config view <" + strings.Join(kinds, "|") + "> [--show-secrets]", "gproxy config view sing-box"},
		map[string]any{"configurations": kinds})
}

// chainModifyGuidance answers a modification that named no chain or nothing to
// change. A chain's tag is fixed: renaming one is a remove and an add, and a
// rule selects it by that tag.
func chainModifyGuidance(missingTag bool) error {
	message := "gproxy route chain modify requires something to change"
	missing := []string{"--parameter", "--dns"}
	if missingTag {
		message = "gproxy route chain modify requires a chain tag"
		missing = append([]string{"<tag>"}, missing...)
	}
	return guidance(message,
		[]string{
			"gproxy route chain modify <tag> [--parameter <host>:<port>[:<username>:<password>]] [--dns <resolver>]",
			"gproxy route chain modify res1 --parameter 198.51.100.7:1080:alice:secret",
		},
		map[string]any{"missing": missing})
}

// chainRemoveGuidance is the one command, flush left; the tags are what
// route chain list prints.
func chainRemoveGuidance() error {
	return guidance("gproxy route chain remove requires a chain tag",
		[]string{"gproxy route chain remove <tag> --confirm"},
		map[string]any{"missing": []string{"<tag>"}})
}

// portGuidance is the one command with the port and its transport written
// together, then one that runs as written.
func portGuidance(action string) error {
	confirm := ""
	if action == "remove" {
		confirm = " --confirm"
	}
	return guidance("gproxy network firewall "+action+" requires <port>/<"+strings.Join(application.FirewallTransports(), "|")+">",
		[]string{
			"gproxy network firewall " + action + " <port>/<" + strings.Join(application.FirewallTransports(), "|") + ">" + confirm,
			"gproxy network firewall " + action + " 8443/tcp" + confirm,
		},
		map[string]any{"missing": []string{"<port>/<transport>"}, "transports": application.FirewallTransports()})
}

// strategyGuidance answers `route direct set` with the closed set of
// strategies, written into the command itself. Cobra's own "accepts 1 arg(s),
// received 0" names neither the argument nor the values it can take. The
// rejected value is not echoed: the command already says what was expected.
func strategyGuidance(args []string) error {
	message := "gproxy route direct set requires a strategy"
	switch {
	case len(args) > 1:
		message = "gproxy route direct set takes one strategy"
	case len(args) == 1:
		message = "unknown direct strategy"
	}
	strategies := application.DirectStrategies()
	return guidance(message,
		[]string{"gproxy route direct set <" + strings.Join(strategies, "|") + ">", "gproxy route direct set auto"},
		map[string]any{"strategies": strategies})
}

// logGuidance answers a bare `gproxy log` with the signature and one command
// that can be run as written. Defaulting to sing-box was a guess at which
// service the reader meant, and a wrong guess reads as a working command.
//
// The selectors are not listed: shell completion offers them, the example names
// one, and two lines is the whole answer. Hint carries the human form and
// JSONMessage the machine one, so a caller parsing the envelope still gets a
// sentence and the full selector list in Data.
func logGuidance() error {
	return &application.Error{
		Code: "invalid_argument",
		Hint: []string{
			"gproxy log <service> [--lines <count>] [--max-bytes <bytes>] [--follow]",
			"gproxy log sing-box --lines 100",
		},
		JSONMessage: "gproxy log requires a service",
		Data:        map[string]any{"services": application.ManagedServiceNames()},
	}
}

// finalGuidance answers `route final set` without a single target.
func finalGuidance(args []string) error {
	message := "gproxy route final set requires a target"
	if len(args) > 1 {
		message = "gproxy route final set takes one target"
	}
	return guidance(message,
		[]string{"gproxy route final set <direct|chain tag>", "gproxy route final set res1", "gproxy route final set direct"},
		map[string]any{"missing": []string{"<direct|chain tag>"}})
}
