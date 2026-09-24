package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go-proxy/internal/application"
	"go-proxy/internal/routing"
)

func guidanceRunner(t *testing.T, out, stderr *bytes.Buffer) *Runner {
	t.Helper()
	r := New("test", "test", strings.NewReader(""), out, stderr)
	r.App.RequireRoot = false
	r.App.LockDir = filepath.Join(t.TempDir(), "absent")
	return r
}

// Guidance must be indistinguishable from any other usage error to a caller:
// stderr, exit 2, nothing on stdout, and no runtime state created. Otherwise an
// agent could read an example command as the command's result.
func TestGuidanceIsAUsageErrorNotAResult(t *testing.T) {
	for _, args := range [][]string{
		{"protocol", "add"},
		{"protocol", "add", "vless"},
		{"server", "restart"},
		{"server", "start"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := guidanceRunner(t, &out, &stderr)
			if code := r.Run(context.Background(), args); code != 2 {
				t.Fatalf("exit=%d, want 2; stderr=%s", code, stderr.String())
			}
			if out.Len() != 0 {
				t.Fatalf("guidance reached stdout: %q", out.String())
			}
			// Guidance is the commands alone, flush left: the error line that
			// once headed them repeated what they already say.
			for _, line := range strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n") {
				if !strings.HasPrefix(line, "gproxy ") {
					t.Fatalf("guidance line is not a command: %q", line)
				}
			}
			if r.executed {
				t.Fatalf("guidance claimed the operation started: %q", stderr.String())
			}
		})
	}
}

func TestGuidanceCarriesChoicesUnderJSON(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"protocol", "add", "--json"}); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Protocols []map[string]string `json:"protocols"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("guidance envelope did not parse: %v: %s", err, out.String())
	}
	if envelope.OK || envelope.Error.Code != "invalid_argument" {
		t.Fatalf("guidance was not an invalid_argument failure: %s", out.String())
	}
	if len(envelope.Data.Protocols) != len(protocolCatalogue) {
		t.Fatalf("machine readers got %d protocols, want %d: %s", len(envelope.Data.Protocols), len(protocolCatalogue), out.String())
	}
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatalf("guidance JSON carried an escape sequence: %q", out.String())
	}
}

// The no-argument guidance is a command reference, not a description of each
// protocol: the reader is assumed to know what VLESS is and to need its flags.
func TestProtocolGuidanceIsACommandReference(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	r.Run(context.Background(), []string{"protocol", "add"})
	text := stderr.String()
	for _, entry := range protocolCatalogue {
		if !strings.Contains(text, entry.Name) {
			t.Fatalf("guidance omits %q:\n%s", entry.Name, text)
		}
		// The forms are already visible in the flags column; a bracketed name
		// beside a command reads as something to type.
		if entry.display() != entry.Name && strings.Contains(text, entry.display()) {
			t.Fatalf("command reference used the display form %q", entry.display())
		}
		if !strings.Contains(text, entry.Usage) {
			t.Fatalf("guidance omits the flags for %q:\n%s", entry.Name, text)
		}
		if entry.Summary != "" && strings.Contains(text, entry.Summary) {
			t.Fatalf("guidance still carries the prose summary for %q", entry.Name)
		}
	}
	// Required flags must appear for every protocol, or the reference is
	// incomplete for the one case every install needs.
	for _, entry := range protocolCatalogue {
		for _, required := range []string{"--user", "--port"} {
			if !strings.Contains(entry.Usage, required) {
				t.Fatalf("%s usage omits %s: %q", entry.Name, required, entry.Usage)
			}
		}
	}
}

// Anything printed as a command must be one. display() renders "vless(tls/
// reality)", which the parser rejects, so it may label a row but never appear
// in a line meant to be typed.
func TestGuidanceCommandLinesAreRunnable(t *testing.T) {
	for _, entry := range protocolCatalogue {
		var out, stderr bytes.Buffer
		r := guidanceRunner(t, &out, &stderr)
		r.Run(context.Background(), []string{"protocol", "add", entry.Name})
		for _, line := range strings.Split(stderr.String(), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "gproxy protocol add ") {
				continue
			}
			if strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")") {
				if strings.Contains(trimmed, entry.display()) {
					t.Fatalf("a line meant to be typed contains the display form: %q", trimmed)
				}
			}
		}
	}
}

// The guidance is one line: the command that completes what was started, with
// every option it takes. It used to be followed by examples, each of which was
// that same line with its brackets resolved one way. If this line stops being
// runnable the guidance is actively misleading, so it is checked per protocol.
func TestInstallGuidanceIsOneRunnableUsageLine(t *testing.T) {
	for _, entry := range protocolCatalogue {
		err := installGuidance(entry, []string{"--port"})
		var detail *application.Error
		if !errors.As(err, &detail) {
			t.Fatalf("%s guidance is not an application error: %v", entry.Name, err)
		}
		if len(detail.Hint) != 1 {
			t.Fatalf("%s guidance is %d lines, want 1:\n%s", entry.Name, len(detail.Hint), strings.Join(detail.Hint, "\n"))
		}
		line := detail.Hint[0]
		if line != "gproxy protocol add "+entry.Name+" "+entry.Usage {
			t.Fatalf("%s guidance is not its own usage line: %q", entry.Name, line)
		}
		// Unindented, so it can be taken off the screen as it stands.
		if strings.HasPrefix(line, " ") {
			t.Fatalf("%s guidance is indented: %q", entry.Name, line)
		}
		for _, required := range []string{"--user ", "--port "} {
			if !strings.Contains(line, required) {
				t.Fatalf("%s guidance omits %s: %q", entry.Name, required, line)
			}
		}
		// The machine answer still carries the same choices.
		data, _ := detail.Data.(map[string]any)
		if data["usage"] != entry.Usage || data["protocol"] != entry.Name {
			t.Fatalf("%s guidance data lost its usage: %#v", entry.Name, data)
		}
	}
}

func TestChainGuidanceExplainsTagAndParameter(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"route", "chain", "add"}); code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	text := stderr.String()
	for _, want := range []string{"<tag>", "--parameter", "<host>:<port>", "<username>:<password>", "[--dns <resolver>]", "[2001:db8::1]:1080", "--dns https://"} {
		if !strings.Contains(text, want) {
			t.Fatalf("chain guidance omits %q:\n%s", want, text)
		}
	}
	// Every hint line is flush left, as the install guidance is.
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if !strings.HasPrefix(line, "gproxy route chain add ") {
			t.Fatalf("chain guidance line is not a flush-left command: %q", line)
		}
	}
	// The guidance passes through the same redaction as any other stderr line.
	// An example that shows a key/value pair gets mangled into <redacted>, which
	// reads as censorship and would be copied literally.
	if strings.Contains(text, "<redacted>") {
		t.Fatalf("guidance example was mangled by redaction:\n%s", text)
	}
}

// The preset guidance is flush left: the error, one command that runs as
// written, then the menu in shell-proxy's numbering with every preset's label.
func TestPresetGuidanceNumbersEveryPresetFlushLeft(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	r.Run(context.Background(), []string{"route", "rule", "add"})
	lines := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n")
	// The example shows several indexes at once, which is what the error line
	// used to say.
	if lines[0] != "gproxy route rule add --user alice --rules 1,3,5,9,a,b --out direct" {
		t.Fatalf("first line is not the example command: %q", lines[0])
	}
	menu := routing.PresetMenu()
	if len(lines) != 1+len(menu) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), 1+len(menu), stderr.String())
	}
	for index, choice := range menu {
		if want := choice.Symbol + "." + choice.Preset.Label; lines[1+index] != want {
			t.Fatalf("menu line %d = %q, want %q", index+1, lines[1+index], want)
		}
	}
}

// Every selectable preset has exactly one index, and the first ones are the
// shell-proxy numbering a reader already knows.
func TestPresetMenuCoversEveryPresetOnce(t *testing.T) {
	seen := map[string]bool{}
	symbols := map[string]bool{}
	for _, choice := range routing.PresetMenu() {
		if seen[choice.Preset.Name] || symbols[choice.Symbol] {
			t.Fatalf("duplicate menu entry %+v", choice)
		}
		seen[choice.Preset.Name], symbols[choice.Symbol] = true, true
	}
	for _, preset := range routing.BuiltinPresets() {
		if preset.Name != "custom" && !seen[preset.Name] {
			t.Fatalf("preset %q has no index", preset.Name)
		}
	}
	for selector, want := range map[string]string{"1": "openai", "9": "github", "g": "discord", "A": "ai-intl", "r": "ads", " t ": "tiktok"} {
		if got, ok := routing.ResolvePresetSelector(selector); !ok || got != want {
			t.Fatalf("selector %q = %q %v, want %q", selector, got, ok, want)
		}
	}
	// Names are not selectors: one numbering is used everywhere.
	for _, selector := range []string{"0", "10", "c", "custom", "", "z", "netflix", "openai"} {
		if _, ok := routing.ResolvePresetSelector(selector); ok {
			t.Fatalf("selector %q was accepted", selector)
		}
	}
}

// modify and remove say which rules the same way: flush-left commands taking
// several comma-separated indexes, and remove's example carries --confirm.
func TestRuleSelectGuidanceIsFlushLeftAndMultiSelect(t *testing.T) {
	for action, want := range map[string]string{
		"remove": "gproxy route rule remove --user alice --rules 1,3,a --confirm",
		"modify": "gproxy route rule modify --user alice --rules 1,3,a --out res1",
	} {
		var out, stderr bytes.Buffer
		r := guidanceRunner(t, &out, &stderr)
		args := []string{"route", "rule", action}
		if action == "remove" {
			args = append(args, "--confirm")
		}
		r.Run(context.Background(), args)
		text := stderr.String()
		if !strings.Contains(text, want) || strings.Contains(text, "error:") {
			t.Fatalf("%s guidance:\n%s", action, text)
		}
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			if !strings.HasPrefix(line, "gproxy route rule ") {
				t.Fatalf("%s guidance line is not a flush-left command: %q", action, line)
			}
		}
	}
}

func TestServerGuidanceListsSelectableServices(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	r.Run(context.Background(), []string{"server", "stop"})
	for _, name := range []string{"sing-box", "proxy-watchdog", "--all"} {
		if !strings.Contains(stderr.String(), name) {
			t.Fatalf("guidance omitted %q:\n%s", name, stderr.String())
		}
	}
}

func TestServiceVerbsAreNoLongerTopLevel(t *testing.T) {
	for _, verb := range []string{"start", "stop", "restart"} {
		var out, stderr bytes.Buffer
		r := guidanceRunner(t, &out, &stderr)
		if code := r.Run(context.Background(), []string{verb, "--all"}); code != 2 {
			t.Fatalf("%s is still reachable at the top level: exit=%d", verb, code)
		}
	}
}

// init and watchdog are hidden because nobody picks them from a list, but they
// are load-bearing: install.sh ends with `gproxy init`, and
// proxy-watchdog.service has `gproxy watchdog` as its ExecStart. Hidden makes
// them easy to mistake for dead code, so this pins that they still resolve.
func TestMachineEntryPointsStayCallableWhileHidden(t *testing.T) {
	var out, stderr bytes.Buffer
	root := New("test", "test", strings.NewReader(""), &out, &stderr).Root()
	for _, name := range []string{"init", "watchdog"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == nil || cmd.Name() != name {
			t.Fatalf("%q no longer resolves: systemd or the installer would break", name)
		}
		if !cmd.Hidden {
			t.Fatalf("%q is listed in help; it is a machine entry point", name)
		}
		if cmd.Short == "" {
			t.Fatalf("%q has no description for its own --help", name)
		}
	}
}

// Running the watchdog by hand occupies the terminal, which reads as a hang.
// Its long help has to say so, and say where to look instead.
func TestWatchdogHelpExplainsWhatItIs(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	if code := r.Run(context.Background(), []string{"watchdog", "--help"}); code != 0 {
		t.Fatalf("watchdog --help exited %d", code)
	}
	for _, want := range []string{"proxy-watchdog.service", "gproxy log proxy-watchdog", "gproxy server start"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("watchdog help does not mention %q:\n%s", want, out.String())
		}
	}
}

// The catalogue is a quick reference: the name and the forms it comes in, not a
// sentence each. It reaches a caller through the guidance envelope now that
// `protocol show` is gone, and the terse spelling is what that envelope carries.
func TestCatalogueIsTerseInTheGuidanceEnvelope(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"protocol", "add", "--json"}); code != 2 {
		t.Fatalf("exit=%d: %s", code, stderr.String())
	}
	var envelope struct {
		Data struct {
			Protocols []struct {
				Name    string `json:"name"`
				Display string `json:"display"`
			} `json:"protocols"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: %v: %s", err, out.String())
	}
	got := []string{}
	for _, protocol := range envelope.Data.Protocols {
		got = append(got, protocol.Display)
	}
	want := []string{"vless(tls/reality)", "tuic", "anytls", "snell(v6/tls)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("catalogue is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The command that carried the catalogue is gone, not renamed and not aliased.
func TestProtocolShowIsGone(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"protocol", "show"}); code != 2 {
		t.Fatalf("protocol show exited %d, want 2: %s%s", code, out.String(), stderr.String())
	}
	if out.Len() != 0 {
		t.Fatalf("protocol show wrote to stdout: %q", out.String())
	}
}

// A variant named in the quick reference must correspond to something the
// protocol actually offers. Not every variant has its own flag — vless's tls is
// the default form, reached by giving --domain — so this looks for the variant
// anywhere the entry describes itself rather than in the option list alone.
func TestEveryAdvertisedVariantIsRealForItsProtocol(t *testing.T) {
	for _, entry := range protocolCatalogue {
		described := strings.ToLower(entry.Summary + " " + entry.Usage + " " + strings.Join(entry.Options, " "))
		for _, variant := range entry.Variants {
			if !strings.Contains(described, strings.ToLower(variant)) {
				t.Fatalf("%s advertises %q, which nothing in its entry mentions", entry.Name, variant)
			}
		}
	}
}

// protocol remove lists what exists, which means its validator reads the store.
// On a host with no runtime that read fails, and the failure must surface as
// itself rather than as a panic or an empty list that implies nothing is
// installed.
func TestRemoveGuidanceSurvivesAnUninitialisedHost(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	code := r.Run(context.Background(), []string{"protocol", "remove"})
	if code == 0 {
		t.Fatalf("exit=0 with no tag given; stdout=%s", out.String())
	}
	if strings.Contains(stderr.String(), "empty: no protocol nodes") {
		t.Fatalf("an unreadable store was reported as an empty one:\n%s", stderr.String())
	}
}

// Destructive commands take --confirm. --yes was the old spelling and must be
// gone, not quietly accepted alongside it.
func TestDestructiveCommandsTakeConfirmAndNotYes(t *testing.T) {
	root := New("test", "test", strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}).Root()
	if root.PersistentFlags().Lookup("confirm") == nil {
		t.Fatal("--confirm is not registered")
	}
	if root.PersistentFlags().Lookup("yes") != nil {
		t.Fatal("--yes still exists; two spellings for one gate is worse than either")
	}
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"protocol", "remove", "some-tag", "--yes"}); code != 2 {
		t.Fatalf("--yes exited %d, want 2 as an unknown flag", code)
	}
}

// With nothing to choose from there is nothing to explain, so the human answer
// is the short form the listing uses for the same state. The envelope still has
// to carry a sentence, because a caller parsing it cannot act on "no nodes".
func TestEmptyRemovalIsShortForPeopleAndASentenceForCallers(t *testing.T) {
	fixture := initializedRuntimeFixture(t)

	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.RequireRoot = false
	r.App.LockDir = fixture
	if code := r.Run(context.Background(), []string{"protocol", "remove"}); code != 2 {
		t.Fatalf("exit=%d, want 2; stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "no nodes" {
		t.Fatalf("human output is %q, want exactly \"no nodes\"", stderr.String())
	}

	out.Reset()
	stderr.Reset()
	r = New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.RequireRoot = false
	r.App.LockDir = fixture
	r.Run(context.Background(), []string{"protocol", "remove", "--json"})
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("envelope did not parse: %v: %s", err, out.String())
	}
	if envelope.Error.Code != "invalid_argument" {
		t.Fatalf("code is %q", envelope.Error.Code)
	}
	if envelope.Error.Message == "" || envelope.Error.Message == "empty" {
		t.Fatalf("machine message is %q; a caller cannot act on that", envelope.Error.Message)
	}
}

// An unknown strategy must list the ones that exist. It is a closed set of
// five, so answering "unknown" without naming them leaves the reader guessing,
// and the rejection has to land before the mutation progress line.
func TestUnknownDirectStrategyListsTheClosedSet(t *testing.T) {
	for _, args := range [][]string{
		{"route", "direct", "set"},
		{"route", "direct", "set", "ipv4"},
		{"route", "direct", "set", "asis", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := guidanceRunner(t, &out, &stderr)
			if code := r.Run(context.Background(), args); code != 2 {
				t.Fatalf("exit %d, want 2; stderr=%s", code, stderr.String())
			}
			text := stderr.String()
			for _, strategy := range application.DirectStrategies() {
				if !strings.Contains(text, strategy) {
					t.Fatalf("guidance omits %q:\n%s", strategy, text)
				}
			}
			// Nothing ran, so no progress line may have been printed.
			if r.executed {
				t.Fatalf("rejected argument still started the operation:\n%s", text)
			}
			if out.Len() != 0 {
				t.Fatalf("guidance reached stdout: %s", out.String())
			}
		})
	}
}

// A bare `gproxy log` used to read sing-box. That was a guess at which service
// the reader meant, and a wrong guess looks like a working command.
func TestBareLogAnswersWithTheSignatureAndOneExample(t *testing.T) {
	var out, stderr bytes.Buffer
	r := guidanceRunner(t, &out, &stderr)
	if code := r.Run(context.Background(), []string{"log"}); code != 2 {
		t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	if out.Len() != 0 {
		t.Fatalf("guidance reached stdout: %s", out.String())
	}
	lines := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("guidance is %d lines, want the signature and one example:\n%s", len(lines), stderr.String())
	}
	if lines[0] != "gproxy log <service> [--lines <count>] [--max-bytes <bytes>] [--follow]" {
		t.Fatalf("first line is not the signature: %q", lines[0])
	}
	// The second line has to be runnable as written.
	if !strings.HasPrefix(lines[1], "gproxy log ") || strings.ContainsAny(lines[1], "<>[]") {
		t.Fatalf("second line is not a command that can be run: %q", lines[1])
	}
	for _, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "error:") {
			t.Fatalf("guidance line carries a prefix or indent: %q", line)
		}
	}

	// A caller parsing the envelope still gets a sentence and every selector.
	out.Reset()
	stderr.Reset()
	r = guidanceRunner(t, &out, &stderr)
	r.Run(context.Background(), []string{"log", "--json"})
	var envelope struct {
		Data  struct{ Services []string } `json:"data"`
		Error struct{ Message string }    `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("envelope: %v: %s", err, out.String())
	}
	if envelope.Error.Message == "" {
		t.Fatalf("machine reader got no message: %s", out.String())
	}
	if len(envelope.Data.Services) != len(application.ManagedServiceNames()) {
		t.Fatalf("envelope omits the selectors: %s", out.String())
	}
}

// A protocol name the catalogue does not carry gets the catalogue, not a bare
// rejection: "ss" was installable until v0.3.1, so the reader needs to see
// what replaced their choice rather than guess at a typo.
func TestUnknownProtocolAnswersWithTheCatalogue(t *testing.T) {
	for _, name := range []string{"ss", "shadowsocks", "nonsense"} {
		var out, stderr bytes.Buffer
		r := guidanceRunner(t, &out, &stderr)
		if code := r.Run(context.Background(), []string{"protocol", "add", name, "--user", "alice", "--port", "auto"}); code != 2 {
			t.Fatalf("%s: exit %d, want 2; stderr=%s", name, code, stderr.String())
		}
		text := stderr.String()
		for _, entry := range protocolCatalogue {
			if !strings.Contains(text, entry.Name) {
				t.Fatalf("%s: guidance omits %q:\n%s", name, entry.Name, text)
			}
		}
		if r.executed {
			t.Fatalf("%s: a withdrawn protocol still started the operation:\n%s", name, text)
		}
	}
}

// The removal guidance lays a node out the way the listing does. The first
// column is the node's name without its port -- the port is the next column --
// and the row number is what the command takes.
func TestRemovalGuidanceUsesTheListingColumns(t *testing.T) {
	err := removeGuidance([]application.ProtocolNode{
		{Tag: "anytls_443", Type: "anytls", Port: 443, Security: "tls", Users: []string{"dhwang1"}},
		{Tag: "snell-v6", Type: "snell", Port: 1443, Security: "none", Users: []string{"dhwang1"},
			ShadowTLS: &application.ProtocolWrapper{Port: 8443}},
		{Tag: "vless_reality_2053", Type: "vless", Port: 2053, Security: "reality"},
	})
	var detail *application.Error
	if !errors.As(err, &detail) {
		t.Fatalf("guidance is not an application error: %v", err)
	}
	text := strings.Join(detail.Hint, "\n")
	for _, want := range []string{
		"1.anytls", "443", "tls", "dhwang1",
		"2.snell-v6", "1443", "shadow-tls",
		"3.vless_reality", "2053", "reality", "no users",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("removal guidance is missing %q:\n%s", want, text)
		}
	}
	// Neither the wrapper's port nor the port already in the next column is
	// repeated in the name.
	if strings.Contains(text, "8443") {
		t.Fatalf("removal guidance names the wrapper port:\n%s", text)
	}
	for _, unwanted := range []string{"anytls_443", "vless_reality_2053"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("the port is still repeated in the name (%q):\n%s", unwanted, text)
		}
	}
	// The row number is what the command takes, and the tag still works.
	if !strings.Contains(text, "<index|tag>") {
		t.Fatalf("the usage line does not offer the row number:\n%s", text)
	}
	if detail.Hint[len(detail.Hint)-1] != strings.TrimLeft(detail.Hint[len(detail.Hint)-1], " ") {
		t.Fatalf("the usage line is indented: %q", detail.Hint[len(detail.Hint)-1])
	}
	// Every column after the tag starts at the same offset, and no line trails
	// whitespace. Measured from where each field begins rather than from the
	// padding that precedes it.
	rows := detail.Hint[:len(detail.Hint)-1]
	columns := []map[int]bool{{}, {}, {}}
	for _, line := range rows {
		if line != strings.TrimRight(line, " ") {
			t.Fatalf("guidance line ends in whitespace: %q", line)
		}
		for index, field := range regexp.MustCompile(` {2,}`).FindAllStringIndex(line, -1) {
			if index < len(columns) {
				columns[index][field[1]] = true
			}
		}
	}
	for index, starts := range columns {
		if len(starts) != 1 {
			t.Fatalf("column %d starts at offsets %v, so it is ragged:\n%s", index+2, starts, text)
		}
	}
}

// A refusal must not follow a claim that the work started. The progress line is
// printed for every mutation before its handler runs, so anything that can
// refuse -- a missing --confirm, a name the rules reject, an argument that is
// not there -- has to be checked as an argument instead.
func TestRefusalsPrintNoProgressLine(t *testing.T) {
	fixture := initializedRuntimeFixture(t)
	cases := [][]string{
		{"user", "remove", "alice"},
		{"user", "add", "user one"},
		{"user", "add"},
		{"user", "rename", "alice"},
		{"protocol", "remove", "vless_443"},
		{"route", "rule", "remove", "--user", "alice", "--rules", "1"},
		{"route", "chain", "remove"},
		{"route", "chain", "remove", "relay"},
		{"network", "firewall", "release"},
		{"network", "firewall", "add", "8443"},
		{"network", "firewall", "add"},
		{"network", "firewall", "remove", "8443/tcp"},
		{"uninstall"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := New("test", "test", strings.NewReader(""), &out, &stderr)
			r.App.RequireRoot = false
			r.App.LockDir = fixture
			if code := r.Run(context.Background(), args); code != 2 {
				t.Fatalf("exit=%d, want 2; stderr=%s", code, stderr.String())
			}
			if r.executed {
				t.Fatalf("refusal followed a progress line:\n%s", stderr.String())
			}
			if out.Len() != 0 {
				t.Fatalf("refusal wrote to stdout: %q", out.String())
			}
		})
	}
}

// chain modify and chain remove answer a missing tag the way chain add does:
// the full command, flush left, and for modify one example; no error line.
func TestChainModifyAndRemoveGuidanceIsFlushLeft(t *testing.T) {
	for args, want := range map[string][]string{
		"route chain modify": {
			"gproxy route chain modify <tag> [--parameter <host>:<port>[:<username>:<password>]] [--dns <resolver>]",
			"gproxy route chain modify res1 --parameter 198.51.100.7:1080:alice:secret",
		},
		"route chain remove": {
			"gproxy route chain remove <tag> --confirm",
		},
	} {
		var out, stderr bytes.Buffer
		r := guidanceRunner(t, &out, &stderr)
		if code := r.Run(context.Background(), strings.Fields(args)); code != 2 {
			t.Fatalf("%s: exit %d", args, code)
		}
		if got := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s guidance:\n%s\nwant:\n%s", args, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}
