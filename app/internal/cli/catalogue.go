package cli

import (
	"strings"

	"go-proxy/internal/application"
	"go-proxy/internal/routing"
)

// protocolEntry describes one installable protocol well enough to answer both
// "what can I install" and "how do I install this one" without the reader
// opening documentation or reading a flag list that mostly does not apply.
type protocolEntry struct {
	Name     string
	Summary  string
	Examples [][2]string
	Options  []string
}

var protocolCatalogue = []protocolEntry{
	{
		Name:    "vless",
		Summary: "VLESS over TLS, or Reality without a certificate",
		Examples: [][2]string{
			{"certificate TLS", "gproxy protocol add vless --user alice --port auto --domain example.com"},
			{"Reality", "gproxy protocol add vless --user alice --port auto --reality"},
		},
		Options: []string{"--port <port|auto>", "--domain <certificate domain>", "--reality", "--sni <handshake domain>"},
	},
	{
		Name:    "ss",
		Summary: "Shadowsocks 2022, optionally wrapped in ShadowTLS",
		Examples: [][2]string{
			{"default AES-256", "gproxy protocol add ss --user alice --port auto"},
			{"AES-128", "gproxy protocol add ss --user alice --port auto --method 2022-blake3-aes-128-gcm"},
			{"with ShadowTLS", "gproxy protocol add ss --user alice --port auto --shadow-tls --shadow-tls-port auto"},
		},
		Options: []string{"--port <port|auto>", "--method <2022-blake3-aes-256-gcm|2022-blake3-aes-128-gcm>", "--shadow-tls", "--shadow-tls-port <port|auto>", "--shadow-tls-sni <domain>"},
	},
	{
		Name:    "tuic",
		Summary: "TUIC v5 over UDP, requires a certificate",
		Examples: [][2]string{
			{"bbr congestion", "gproxy protocol add tuic --user alice --port auto --domain example.com"},
			{"cubic congestion", "gproxy protocol add tuic --user alice --port auto --domain example.com --congestion cubic"},
		},
		Options: []string{"--port <port|auto>", "--domain <certificate domain>", "--congestion <bbr|cubic>"},
	},
	{
		Name:    "anytls",
		Summary: "AnyTLS, requires a certificate",
		Examples: [][2]string{
			{"install", "gproxy protocol add anytls --user alice --port auto --domain example.com"},
		},
		Options: []string{"--port <port|auto>", "--domain <certificate domain>"},
	},
	{
		Name:    "snell",
		Summary: "Snell v6, one node per host, optionally wrapped in ShadowTLS",
		Examples: [][2]string{
			{"install", "gproxy protocol add snell --user alice --port auto"},
			{"IPv6 egress", "gproxy protocol add snell --user alice --port auto --ipv6"},
			{"with ShadowTLS", "gproxy protocol add snell --user alice --port auto --shadow-tls --shadow-tls-port auto"},
		},
		Options: []string{"--port <port|auto>", "--ipv6", "--shadow-tls", "--shadow-tls-port <port|auto>", "--shadow-tls-sni <domain>"},
	},
}

// joinList renders a flag list the way a sentence needs it: "--user",
// "--user and --port", "--user, --preset and --out".
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
		entries = append(entries, map[string]string{"name": entry.Name, "summary": entry.Summary})
	}
	return entries
}

// guidance is the failure a command returns when it was named but not given
// enough to act. It is an ordinary invalid_argument, so it reaches stderr at
// exit 2 and a caller cannot read it as a result; Data carries the same choices
// in a form a machine reader can use.
func guidance(message string, hint []string, data any) error {
	return &application.Error{Code: "invalid_argument", Message: message, Hint: hint, Data: data}
}

func chooseProtocolGuidance() error {
	hint := make([]string, 0, len(protocolCatalogue)+1)
	width := 0
	for _, entry := range protocolCatalogue {
		if len(entry.Name) > width {
			width = len(entry.Name)
		}
	}
	for _, entry := range protocolCatalogue {
		hint = append(hint, "  "+pad(entry.Name, width)+"  "+entry.Summary)
	}
	hint = append(hint, "  run: gproxy protocol add <protocol> --user <name> --port <port|auto>")
	return guidance("gproxy protocol add requires a protocol: "+strings.Join(catalogueNames(), ", "),
		hint, map[string]any{"protocols": catalogueData()})
}

func installGuidance(entry protocolEntry, missing []string) error {
	width := 0
	for _, example := range entry.Examples {
		if len(example[0]) > width {
			width = len(example[0])
		}
	}
	hint := make([]string, 0, len(entry.Examples)+2)
	hint = append(hint, "  "+entry.Name+" examples:")
	for _, example := range entry.Examples {
		hint = append(hint, "    "+pad(example[0], width)+"  "+example[1])
	}
	hint = append(hint, "  options: "+strings.Join(entry.Options, "  "))
	return guidance(joinList(missing)+" required for gproxy protocol add "+entry.Name,
		hint, map[string]any{"protocol": entry.Name, "missing": missing, "options": entry.Options})
}

// presetGuidance replaces the separate `routing presets` command: the names are
// only ever needed while composing a rule, which is exactly when this prints.
func presetGuidance(missing []string) error {
	names := []string{}
	for _, preset := range routing.BuiltinPresets() {
		if preset.Name != "custom" {
			names = append(names, preset.Name)
		}
	}
	return guidance(joinList(missing)+" required for gproxy route rule add",
		[]string{
			"  presets: " + strings.Join(names, ", "),
			"  --out:   direct, or a chain tag from gproxy route chain show",
			"  run: gproxy route rule add --user alice --preset ads,github --out direct",
		},
		map[string]any{"missing": missing, "presets": names})
}
