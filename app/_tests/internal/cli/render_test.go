package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"go-proxy/internal/application"
	"go-proxy/internal/cert"
	"go-proxy/internal/core"
	"go-proxy/internal/network"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/update"
)

func statusFields() map[string]any {
	expires := time.Now().Add(89 * 24 * time.Hour)
	return map[string]any{
		"healthy": true, "complete": true, "issues": []string{},
		"users": 2, "nodes": 5, "routing_rules": 12,
		"system":      map[string]any{"os": "linux", "arch": "amd64", "version": "Debian GNU/Linux 12 (bookworm)"},
		"memberships": map[string][]string{"alice": {"anytls", "vless"}, "bob": {"tuic"}, "carol": {}},
		"cert":        cert.Status{Domain: "example.com", Ready: true, ExpiresAt: &expires},
		"network": network.Observation{Addresses: []network.Address{
			{Family: "ipv4", Scope: "loopback", Address: "127.0.0.1/8"},
			{Family: "ipv4", Scope: "private", Address: "10.0.0.2/32"},
			{Family: "ipv6", Scope: "link_local", Address: "fe80::1/64"},
			{Family: "ipv6", Scope: "global", Address: "2001:db8::1/128"},
		}},
		"services": []service.Status{
			{Name: service.SingBox, Installed: true, Running: true, State: "active"},
			{Name: service.Snell, Installed: true, Running: false, State: "inactive"},
			{Name: service.ShadowTLS, Installed: false},
		},
	}
}

func TestStatusRenderingReportsEveryDashboardRow(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy status", statusFields()) {
		t.Fatal("status was not rendered")
	}
	text := out.String()
	for _, want := range []string{
		"1.system", "Debian GNU/Linux 12 (bookworm)",
		"2.network", "10.0.0.2", "2001:db8::1",
		"3.protocol", "alice:anytls/vless", "bob:tuic",
		"4.service", "sing-box(running)/snell(stopped)/shadow-tls(absent)",
		"5.domain", "example.com", "(expires in 89 days)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered status is missing %q:\n%s", want, text)
		}
	}
	// Rows are numbered and unindented: a leading space would misalign the
	// numbering the whole layout is built around.
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.HasPrefix(line, " ") {
			t.Fatalf("row is indented: %q", line)
		}
	}
	// The prefix length belongs to the subnet, not to the host's identity.
	if strings.Contains(text, "/32") || strings.Contains(text, "/128") {
		t.Fatalf("address kept its prefix length:\n%s", text)
	}
	// Loopback and link-local are not the host's address.
	for _, unwanted := range []string{"127.0.0.1", "fe80::1"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("reported %q as the host address:\n%s", unwanted, text)
		}
	}
	// Service names are the short ones, and the unit suffixes are gone.
	for _, unwanted := range []string{"snell-v6", "caddy-sub", "proxy-watchdog"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("service row used the unit name %q:\n%s", unwanted, text)
		}
	}
}

// A user with no protocols must still appear: the dashboard answers "who has
// what", and silently dropping a user answers it wrongly.
func TestStatusRenderingKeepsUsersWithoutProtocols(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", statusFields())
	if !strings.Contains(out.String(), "carol:none") {
		t.Fatalf("a user without protocols was dropped:\n%s", out.String())
	}
}

// Running, not running and not installed must be visually distinct, or the row
// is a list of names carrying no state at all.
func TestServiceRowColoursTheThreeStates(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{on: true}, "gproxy status", statusFields())
	text := out.String()
	for name, colour := range map[string]string{
		"sing-box":   ansiRunning,
		"snell":      ansiStopped,
		"shadow-tls": ansiUnknown,
	} {
		if !strings.Contains(text, colour+name) {
			t.Fatalf("service %q is not shown in its state colour", name)
		}
	}
	// Colour says it, so the word does not: naming the state twice is noise in
	// the row the dashboard has least space in.
	for _, word := range []string{"(running)", "(stopped)", "(absent)"} {
		if strings.Contains(text, word) {
			t.Fatalf("coloured service row also spells out %s:\n%s", word, text)
		}
	}
}

// Without colour the state has to be a word. A pipe, a file, NO_COLOR and
// --no-color all strip the colour, and the row existed to answer which services
// are running.
func TestServiceRowNamesTheThreeStatesWithoutColour(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", statusFields())
	text := out.String()
	for _, want := range []string{"sing-box(running)", "snell(stopped)", "shadow-tls(absent)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("colour-free service row is missing %q:\n%s", want, text)
		}
	}
}

func TestStatusRenderingSurfacesIssues(t *testing.T) {
	fields := statusFields()
	fields["healthy"] = false
	fields["issues"] = []string{"configured service sing-box is inactive"}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", fields)
	if !strings.Contains(out.String(), "configured service sing-box is inactive") {
		t.Fatalf("issue missing from rendered status:\n%s", out.String())
	}
}

func TestRenderingEmitsNoEscapesWithoutColour(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", statusFields())
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatalf("escape sequence emitted with colour off:\n%q", out.String())
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy status", statusFields())
	if !bytes.Contains(coloured.Bytes(), []byte{0x1b}) {
		t.Fatal("colour enabled but no escape sequence emitted")
	}
	if strings.Count(coloured.String(), ansiReset) < strings.Count(coloured.String(), "\x1b[38") {
		t.Fatal("a colour was opened without being reset")
	}
}

func TestColourIsOffForAnythingButATerminal(t *testing.T) {
	var buffer bytes.Buffer
	if colorEnabled(&buffer, false) {
		t.Fatal("colour enabled for a non-file writer")
	}
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if colorEnabled(file, false) {
		t.Fatal("colour enabled for a regular file")
	}
	t.Setenv("NO_COLOR", "1")
	if colorEnabled(os.Stdout, false) {
		t.Fatal("NO_COLOR was ignored")
	}
}

func TestRenderingStripsControlCharactersFromSystemText(t *testing.T) {
	fields := statusFields()
	fields["issues"] = []string{"unit \x1b[31mred\x1b[0m failed"}
	fields["services"] = []service.Status{{Name: service.Name("sing-box\x1b[5m"), Installed: true, Running: true, State: "active"}}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", fields)
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatalf("escape sequence from system text survived into colour-free output:\n%q", out.String())
	}
	if !strings.Contains(out.String(), "red") || !strings.Contains(out.String(), "sing-box") {
		t.Fatalf("stripping removed legible text:\n%s", out.String())
	}
}

func TestStatusRenderingSkipsRowsWithNoData(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy status", map[string]any{"healthy": true}) {
		t.Fatal("status was not rendered")
	}
	for _, absent := range []string{"nodes", "users", "routes", "system", "cert"} {
		if strings.Contains(out.String(), absent) {
			t.Fatalf("row %q rendered with no data behind it:\n%q", absent, out.String())
		}
	}
}

func TestUnrenderedCommandsFallBackToJSON(t *testing.T) {
	var out bytes.Buffer
	if render(&out, palette{}, "gproxy user", map[string]any{"users": []string{"alice"}}) {
		t.Fatal("a command with no rendering claimed to render")
	}
	if out.Len() != 0 {
		t.Fatalf("fallback path wrote output: %q", out.String())
	}
}

func TestCertRenderingFlagsExpiryStates(t *testing.T) {
	soon := time.Now().Add(3 * 24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	cases := []struct {
		status cert.Status
		want   string
	}{
		{cert.Status{Domain: "a.example", Ready: false}, "not issued"},
		{cert.Status{Domain: "a.example", Ready: true}, "ready"},
		{cert.Status{Domain: "a.example", Ready: true, ExpiresAt: &soon}, "expires in 3 days"},
		{cert.Status{Domain: "a.example", Ready: true, ExpiresAt: &past}, "expired"},
	}
	for _, testCase := range cases {
		if got := renderCert(palette{}, testCase.status); !strings.Contains(got, testCase.want) {
			t.Fatalf("cert rendering %+v produced %q, want %q", testCase.status, got, testCase.want)
		}
	}
}

// assertNumberedLayout checks the one layout every rendering shares: no indent,
// rows numbered from one, and a heading only where a section starts. Sections
// restart the numbering because the commands acting on those rows index within
// the section, not across the whole listing.
func assertNumberedLayout(t *testing.T, text string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("rendering was empty")
	}
	number := 0
	for index, line := range lines {
		switch {
		case strings.HasPrefix(line, " "):
			t.Fatalf("line %d is indented: %q", index+1, line)
		case line == "":
			if number == 0 {
				t.Fatalf("line %d is a blank line outside a section break", index+1)
			}
			number = 0
		case strings.HasPrefix(line, fmt.Sprintf("%d.", number+1)):
			number++
		case number == 0: // a section heading, whose rows are numbered below it
		default:
			t.Fatalf("line %d is neither row %d nor a section heading: %q", index+1, number+1, line)
		}
	}
	if number == 0 {
		t.Fatalf("rendering ended on a heading with no rows:\n%s", text)
	}
}

// Every rendered command shares one layout. Checking it per command would let
// the next renderer quietly diverge, so this asserts the shape itself.
func TestEveryRenderedCommandUsesTheNumberedLayout(t *testing.T) {
	for command, data := range numberedLayoutCases() {
		t.Run(command, func(t *testing.T) {
			var out bytes.Buffer
			if !render(&out, palette{}, command, data) {
				t.Fatal("command produced no rendering")
			}
			assertNumberedLayout(t, out.String())
		})
	}
}

func numberedLayoutCases() map[string]any {
	return map[string]any{
		"gproxy status": statusFields(),
		"gproxy server status": map[string]any{"services": []service.Status{
			{Name: service.SingBox, Installed: true, Running: true, State: "active"},
			{Name: service.CaddySub, Installed: true, Running: false, State: "inactive"},
			{Name: service.Watchdog, Installed: false},
		}},
		"gproxy protocol list": map[string]any{"protocols": catalogueData()},
		"gproxy route chain list": map[string]any{"chains": []application.ChainView{
			{Tag: "relay", Host: "127.0.0.1", Port: 1080, Address: "127.0.0.1:1080", Users: []string{"alice"}},
		}},
		"gproxy route rule list": map[string]any{"rules": []application.RouteEntry{
			{User: "alice", Index: 1, Label: "OpenAI/ChatGPT", Outbound: "relay", Address: "198.51.100.10:1080"},
			{User: "alice", Index: 2, Label: "Google", Outbound: "direct"},
			{User: "bob", Index: 1, Label: "Netflix", Outbound: "relay", Address: "198.51.100.10:1080"},
		}},
		"gproxy route direct list": map[string]any{"strategy": "prefer_ipv6"},
		"gproxy network fail2ban status": network.Fail2BanInfo{
			Installed: true, Running: true, SSHJailEnabled: true,
			MaxRetry: "5", BanTime: "600", FindTime: "600", BannedIPs: []string{"198.51.100.5"},
		},
		// Mutations. Each printed an indented JSON object before, which is the
		// one shape this layout does not cover.
		"gproxy protocol add": application.ProtocolNode{
			Tag: "vless_reality_443", Type: "vless", Port: 443, Security: "reality", Users: []string{"alice"},
		},
		"gproxy protocol remove": map[string]any{"tag": "vless_reality_443", "removed": true, "user": "bob"},
		"gproxy route chain add": application.ChainView{
			Tag: "relay", Host: "127.0.0.1", Port: 1080, Address: "127.0.0.1:1080", DomainStrategy: "ipv4_only",
		},
		"gproxy route chain remove":      map[string]any{"tag": "relay", "removed": true},
		"gproxy route sync-dns":          map[string]any{"strategy": "prefer_ipv4"},
		"gproxy network fail2ban enable": network.Fail2BanInfo{Installed: true, Running: true, SSHJailEnabled: true},
		"gproxy server restart": []service.Status{
			{Name: service.SingBox, Installed: true, Running: true, State: "active"},
		},
		"gproxy core update": map[string]any{"updated": []core.Component{core.CompSingBox}},
		"gproxy init":        map[string]any{"initialized": true, "runtime": "/etc/go-proxy"},
	}
}

// A removal that removed nothing must not read like one that worked. The
// envelope says so in changed; the human rendering has to say it in a word.
func TestRemovalRenderingSeparatesRemovedFromNotFound(t *testing.T) {
	cases := []struct {
		command string
		fields  map[string]any
		want    string
	}{
		{"gproxy protocol remove", map[string]any{"tag": "vless", "removed": false}, "not found"},
		{"gproxy protocol remove", map[string]any{"tag": "vless_443", "removed": true}, "removed"},
		{"gproxy user remove", map[string]any{"user": "ghost", "removed": false}, "not found"},
		{"gproxy user remove", map[string]any{"user": "alice", "removed": true}, "removed"},
		{"gproxy route chain remove", map[string]any{"tag": "ghost", "removed": false}, "not found"},
		{"gproxy network firewall release", map[string]any{"managed": false, "removed": false}, "not applied"},
	}
	for _, testCase := range cases {
		var out bytes.Buffer
		if !render(&out, palette{}, testCase.command, testCase.fields) {
			t.Fatalf("%s produced no rendering", testCase.command)
		}
		if !strings.Contains(out.String(), testCase.want) {
			t.Fatalf("%s on %v rendered %q, want %q", testCase.command, testCase.fields, out.String(), testCase.want)
		}
	}
}

// Service state is two words, not four. "not installed" and "not enabled at
// boot" were detail nobody acted on; the distinction lives in colour instead.
func TestServiceStateIsRunningOrStopped(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy server status", map[string]any{"services": []service.Status{
		{Name: service.SingBox, Installed: true, Running: true, State: "active"},
		{Name: service.Snell, Installed: false},
		{Name: service.CaddySub, Installed: true, Running: false, State: "failed"},
	}})
	text := out.String()
	for _, unwanted := range []string{"not installed", "not enabled", "inactive", "failed", "absent"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("service row still carries %q:\n%s", unwanted, text)
		}
	}
	if strings.Count(text, "stopped") != 2 || !strings.Contains(text, "running") {
		t.Fatalf("expected one running and two stopped:\n%s", text)
	}
	// Unit suffixes are display noise.
	for _, unwanted := range []string{"caddy-sub", "proxy-watchdog", "snell-v6"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("unit name %q leaked into the display:\n%s", unwanted, text)
		}
	}
}

// A rule index is only meaningful inside its user: `route rule remove --user
// bob --rules 1` means bob's first rule. Numbering the listing straight through
// would print indexes that command rejects.
func TestRuleListingNumbersWithinEachUser(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy route rule list", map[string]any{"rules": []application.RouteEntry{
		{User: "alice", Index: 1, Label: "OpenAI/ChatGPT", Outbound: "relay", Address: "198.51.100.10:1080"},
		{User: "alice", Index: 2, Label: "Google", Outbound: "direct"},
		{User: "bob", Index: 1, Label: "Netflix", Outbound: "relay", Address: "198.51.100.10:1080"},
	}}) {
		t.Fatal("rule listing produced no rendering")
	}
	text := out.String()
	assertNumberedLayout(t, text)
	for _, want := range []string{
		"alice:",
		"1.OpenAI/ChatGPT",
		"2.Google",
		"bob:",
		"1.Netflix",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rule listing is missing %q:\n%s", want, text)
		}
	}
	// A rule says where it goes, including the server behind a chain tag: the
	// reader should not need route chain list to finish reading one line.
	if !strings.Contains(text, "-> relay") || !strings.Contains(text, "198.51.100.10:1080") {
		t.Fatalf("rule listing does not report its destination:\n%s", text)
	}
	// Direct egress has no address to report, and an empty column would read as
	// a missing value rather than as the absence of one.
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "Google") && strings.TrimRight(line, " ") != line {
			t.Fatalf("direct rule padded to an empty column: %q", line)
		}
	}
}

func TestEmptyRuleListingSaysSo(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy route rule list", map[string]any{"rules": []application.RouteEntry{}}) {
		t.Fatal("empty rule listing produced no rendering")
	}
	if strings.TrimSpace(out.String()) != "no rules" {
		t.Fatalf("empty rule listing said %q", out.String())
	}
}

// The chain listing is the rule listing read from the other end: it names the
// users whose rules select the chain, which is also what makes a refused
// removal predictable.
func TestChainListingNamesItsUsers(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy route chain list", map[string]any{"chains": []application.ChainView{
		{Tag: "relay", Host: "198.51.100.10", Port: 1080, Address: "198.51.100.10:1080", Authenticated: true, Users: []string{"alice", "bob"}},
		{Tag: "spare", Host: "2001:db8::1", Port: 1080, Address: "[2001:db8::1]:1080", Users: []string{}},
	}}) {
		t.Fatal("chain listing produced no rendering")
	}
	text := out.String()
	assertNumberedLayout(t, text)
	for _, want := range []string{"1.relay", "-> 198.51.100.10:1080", "authenticated", "alice/bob", "2.spare", "[2001:db8::1]:1080"} {
		if !strings.Contains(text, want) {
			t.Fatalf("chain listing is missing %q:\n%s", want, text)
		}
	}
	// An unused chain reports no users rather than an empty separator.
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "spare") && strings.Contains(line, "/") && !strings.Contains(line, "::") {
			t.Fatalf("unused chain rendered a user separator: %q", line)
		}
	}
}

func TestDirectStrategyRendersAsARow(t *testing.T) {
	for _, command := range []string{"gproxy route direct list", "gproxy route direct set", "gproxy route sync-dns"} {
		var out bytes.Buffer
		if !render(&out, palette{}, command, map[string]any{"strategy": "prefer_ipv6"}) {
			t.Fatalf("%s produced no rendering", command)
		}
		if strings.TrimSpace(out.String()) != "1.strategy  prefer_ipv6" {
			t.Fatalf("%s rendered %q", command, out.String())
		}
	}
}

// A log is another program's output. It keeps its own lines, unnumbered and
// unindented, because numbering would break a copied or grepped line and no
// command takes a log line number the way --rules takes a rule index. That is
// why `gproxy log` is absent from the numbered-layout cases above.
func TestLogKeepsItsLinesAndColoursOnlyFailures(t *testing.T) {
	fields := map[string]any{
		"service": "sing-box",
		"source":  "/etc/go-proxy/logs/sing-box.service.log",
		// sing-box colours its own output; the residue of a half-stripped
		// escape is what made this unreadable.
		"content": "\x1b[33mWARN\x1b[0m[0000] deprecated option\nINFO[0001] rule-set loaded\n\x1b[31mERROR\x1b[0m[0002] address already in use\n",
	}
	var plain bytes.Buffer
	if !render(&plain, palette{}, "gproxy log", fields) {
		t.Fatal("log produced no rendering")
	}
	text := plain.String()
	if strings.Contains(text, "\x1b") || strings.Contains(text, "[33m") || strings.Contains(text, "[31m") {
		t.Fatalf("foreign escape survived into colour-free output:\n%q", text)
	}
	// One space after the service, and the log starts on the next line.
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if lines[0] != "sing-box /etc/go-proxy/logs/sing-box.service.log" {
		t.Fatalf("header is not the service and its source:\n%q", text)
	}
	for index, want := range []string{"WARN[0000] deprecated option", "INFO[0001] rule-set loaded", "ERROR[0002] address already in use"} {
		if lines[index+1] != want {
			t.Fatalf("log line %d is %q, want %q", index+1, lines[index+1], want)
		}
	}

	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy log", fields)
	for _, want := range []string{ansiBad + "ERROR[0002]", ansiSys + "WARN[0000]"} {
		if !strings.Contains(coloured.String(), want) {
			t.Fatalf("failure line is not highlighted: %q missing", want)
		}
	}
	// Colouring every line is what stops any of them standing out.
	if strings.Contains(coloured.String(), ansiBad+"INFO") || strings.Contains(coloured.String(), ansiSys+"INFO") {
		t.Fatalf("a routine line was highlighted:\n%q", coloured.String())
	}
}

func TestEmptyLogSaysSo(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy log", map[string]any{"service": "snell-v6", "source": "journal", "content": ""}) {
		t.Fatal("empty log produced no rendering")
	}
	if !strings.Contains(out.String(), "no entries") {
		t.Fatalf("empty log rendered %q", out.String())
	}
}

// A followed log never reaches the renderer, so the same treatment has to be
// applied as the stream arrives -- including across a read that stops mid-line.
func TestFollowWriterColoursLinesAcrossPartialReads(t *testing.T) {
	var out bytes.Buffer
	writer := &logWriter{out: &out, p: palette{on: true}}
	for _, chunk := range []string{"INFO star", "ted\n\x1b[31mERROR", "[1] failed to bind\nWARN tail"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "INFO started\n") {
		t.Fatalf("a line split across reads was not rejoined:\n%q", text)
	}
	if !strings.Contains(text, ansiBad+"ERROR[1] failed to bind") {
		t.Fatalf("failure line not highlighted:\n%q", text)
	}
	// A followed log is cancelled rather than finished, so the last read
	// usually stops mid-line and that remainder must still be written.
	if !strings.Contains(text, ansiSys+"WARN tail") {
		t.Fatalf("trailing partial line was dropped:\n%q", text)
	}
	if strings.Contains(text, "[31m") {
		t.Fatalf("foreign escape passed through the follow path:\n%q", text)
	}
}

// A log line with no newline in it must not be buffered without limit.
func TestFollowWriterFlushesAnUnterminatedLine(t *testing.T) {
	var out bytes.Buffer
	writer := &logWriter{out: &out, p: palette{}}
	if _, err := writer.Write(bytes.Repeat([]byte("x"), logLineCap+1)); err != nil {
		t.Fatal(err)
	}
	if out.Len() != logLineCap+1 {
		t.Fatalf("wrote %d bytes of an oversized line, want %d", out.Len(), logLineCap+1)
	}
	if len(writer.partial) != 0 {
		t.Fatalf("held %d bytes after flushing", len(writer.partial))
	}
}

// Unapplied, the status lists the ports gproxy would keep open and ends on
// the command that applies them, saying what applying does to the rest.
func TestFirewallStatusListsWhatApplyWouldOpen(t *testing.T) {
	info := network.FirewallInfo{
		Available: true,
		Desired: []network.FirewallPortSpec{
			{Proto: "tcp", Port: 22, Sources: []string{"ssh"}},
			{Proto: "tcp", Port: 443, Sources: []string{"anytls", "caddy"}},
			{Proto: "udp", Port: 28218, Sources: []string{"tuic"}},
		},
		Add: []network.CurrentPortEntry{{Proto: "tcp", Port: 22}, {Proto: "tcp", Port: 443}, {Proto: "udp", Port: 28218}},
	}
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy network firewall status", info) {
		t.Fatal("firewall status produced no rendering")
	}
	want := "firewall  not applied  nftables available\n" +
		"22/tcp     ssh\n" +
		"443/tcp    anytls, caddy\n" +
		"28218/udp  tuic\n" +
		"gproxy network firewall apply   (opens these 3 ports, drops other inbound)\n"
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}

	// Applied and in sync: the same ports, and nothing to run.
	info.Managed, info.Add = true, nil
	info.Rules = "table inet proxy_firewall {\n  chain input {\n  }\n}\n"
	var synced bytes.Buffer
	render(&synced, palette{}, "gproxy network firewall apply", info)
	if !strings.HasPrefix(synced.String(), "firewall  applied  in sync\n22/tcp") || strings.Contains(synced.String(), "gproxy network firewall apply") {
		t.Fatalf("in sync:\n%s", synced.String())
	}
	// nft's own syntax stays in the JSON.
	if strings.Contains(synced.String(), "chain input") {
		t.Fatalf("the nftables ruleset was reprinted:\n%s", synced.String())
	}
}

// With drift, only the changes apply would make are listed, marked + and -.
func TestFirewallStatusShowsOnlyTheDrift(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy network firewall status", network.FirewallInfo{
		Available: true, Managed: true,
		Desired: []network.FirewallPortSpec{
			{Proto: "tcp", Port: 22, Sources: []string{"ssh"}},
			{Proto: "tcp", Port: 8443, Sources: []string{"custom"}},
		},
		Current: []network.CurrentPortEntry{{Proto: "tcp", Port: 22, Action: "accept"}, {Proto: "udp", Port: 9000, Action: "accept"}},
		Add:     []network.CurrentPortEntry{{Proto: "tcp", Port: 8443}},
		Remove:  []network.CurrentPortEntry{{Proto: "udp", Port: 9000}},
	})
	want := "firewall  applied  2 changes pending\n" +
		"+ 8443/tcp  custom\n" +
		"- 9000/udp  no longer used\n" +
		"gproxy network firewall apply\n"
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}
}

// add and remove answer per port, one line each; "both" is two.
func TestFirewallPortChangesArePerPort(t *testing.T) {
	cases := []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"managed": true, "changes": []application.FirewallPortChange{
			{Port: 8443, Transport: "tcp", Result: "added"}, {Port: 8443, Transport: "udp", Result: "already added"},
		}}, "8443/tcp  custom  (added)\n8443/udp  custom  (already added)\n"},
		{map[string]any{"managed": false, "changes": []application.FirewallPortChange{{Port: 8443, Transport: "tcp", Result: "added"}}},
			"8443/tcp  custom  (added; firewall not applied)\n"},
		{map[string]any{"managed": true, "changes": []application.FirewallPortChange{
			{Port: 8443, Transport: "tcp", Result: "removed"}, {Port: 8443, Transport: "udp", Result: "not found"},
		}}, "8443/tcp  custom (removed)\n8443/udp  custom  (not found)\n"},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if !render(&out, palette{}, "gproxy network firewall add", c.fields) || out.String() != c.want {
			t.Fatalf("got %q, want %q", out.String(), c.want)
		}
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy network firewall remove", map[string]any{"managed": true,
		"changes": []application.FirewallPortChange{{Port: 8443, Transport: "tcp", Result: "removed"}}})
	if !strings.HasPrefix(coloured.String(), "\x1b[9m") {
		t.Fatalf("a removed port is not struck through: %q", coloured.String())
	}
	var released bytes.Buffer
	render(&released, palette{}, "gproxy network firewall release", map[string]any{"managed": false, "removed": true})
	if released.String() != "firewall  removed\n" {
		t.Fatalf("release = %q", released.String())
	}
}

// The v0.1.59 table reported the thresholds that decide a ban and who is
// banned now. Dropping them left a status that could not explain a ban.
func TestFail2banStatusReportsThresholdsAndBannedAddresses(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy network fail2ban status", network.Fail2BanInfo{
		Installed: true, Running: true, SSHJailEnabled: true,
		MaxRetry: "5", BanTime: "600", FindTime: "600",
		CurrentlyBanned: 2, TotalBanned: 17,
		BannedIPs: []string{"198.51.100.5", "198.51.100.9"},
	})
	text := out.String()
	assertNumberedLayout(t, text)
	for _, want := range []string{"max retry", "5", "ban time", "600s", "find time", "banned now", "banned total", "17", "198.51.100.5/198.51.100.9"} {
		if !strings.Contains(text, want) {
			t.Fatalf("fail2ban status is missing %q:\n%s", want, text)
		}
	}
	// Absent thresholds are not rendered as empty rows.
	var bare bytes.Buffer
	render(&bare, palette{}, "gproxy network fail2ban status", network.Fail2BanInfo{})
	for _, unwanted := range []string{"max retry", "ban time", "find time", "banned ip"} {
		if strings.Contains(bare.String(), unwanted) {
			t.Fatalf("uninstalled fail2ban rendered %q:\n%s", unwanted, bare.String())
		}
	}
}

// Dropping only the escape byte left "[33m" in front of every line sing-box
// coloured. The whole sequence has to go.
func TestCleanRemovesWholeEscapeSequences(t *testing.T) {
	for _, testCase := range []struct{ in, want string }{
		{"\x1b[33mWARN\x1b[0m ready", "WARN ready"},
		{"\x1b[38;2;1;2;3mdeep\x1b[0m", "deep"},
		{"plain", "plain"},
		{"tab\there", "tabhere"},
		{"\x1b", ""},
		{"\x1b[", ""},
		{"unicode 日本語", "unicode 日本語"},
	} {
		if got := clean(testCase.in); got != testCase.want {
			t.Fatalf("clean(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}

// Padding the last column on a line leaves trailing whitespace, which shows up
// in diffs and copied output. Colour hides it from the eye, not from the file.
func TestRenderedLinesNeverEndInWhitespace(t *testing.T) {
	for _, coloured := range []bool{false, true} {
		for command, data := range numberedLayoutCases() {
			var out bytes.Buffer
			render(&out, palette{on: coloured}, command, data)
			for _, line := range strings.Split(out.String(), "\n") {
				bare := strings.ReplaceAll(line, ansiReset, "")
				for _, code := range []string{ansiOK, ansiBad, ansiSys, ansiCount, ansiLabel, ansiHint, ansiRunning, ansiStopped, ansiUnknown} {
					bare = strings.ReplaceAll(bare, code, "")
				}
				if bare != strings.TrimRight(bare, " ") {
					t.Fatalf("%s (colour=%v) line ends in whitespace: %q", command, coloured, line)
				}
			}
		}
	}
}

// The dashboard has one network row, not a local one and a public one. It
// lists the addresses this host has, the probed ones included.
func TestNetworkRowListsEveryAddressOnce(t *testing.T) {
	probed := func(ipv4, ipv6 network.Probe) map[string]any {
		fields := statusFields()
		observed := fields["network"].(network.Observation)
		observed.IPv4, observed.IPv6 = ipv4, ipv6
		fields["network"] = observed
		return fields
	}
	unset := network.Probe{State: "not_checked"}

	var plain bytes.Buffer
	render(&plain, palette{}, "gproxy status", statusFields())
	if strings.Contains(plain.String(), "public") {
		t.Fatalf("the public row outlived the merge:\n%s", plain.String())
	}

	// Behind NAT the private address is not one anyone reaches the host on:
	// the probed address replaces it.
	var nat bytes.Buffer
	render(&nat, palette{}, "gproxy status", probed(network.Probe{State: "available", Address: "203.0.113.7"}, unset))
	for _, want := range []string{"203.0.113.7", "2001:db8::1"} {
		if !strings.Contains(nat.String(), want) {
			t.Fatalf("network row is missing %q:\n%s", want, nat.String())
		}
	}
	if strings.Contains(nat.String(), "10.0.0.2") {
		t.Fatalf("the internal address was shown beside the external one:\n%s", nat.String())
	}
	assertNumberedLayout(t, nat.String())

	// With no probe answer the private address is all there is, and it says so.
	if !strings.Contains(plain.String(), "10.0.0.2 (internal)") {
		t.Fatalf("an unprobed private address is not marked internal:\n%s", plain.String())
	}

	// An untranslated family returns the same address from the interface and
	// from the probe; printing it twice would read as two addresses.
	var direct bytes.Buffer
	render(&direct, palette{}, "gproxy status", probed(unset, network.Probe{State: "available", Address: "2001:db8::1"}))
	if strings.Count(direct.String(), "2001:db8::1") != 1 {
		t.Fatalf("an untranslated address was printed twice:\n%s", direct.String())
	}
}

// A host that has been scanned for a week bans hundreds. The count says how
// many; the row shows enough to recognise one and leaves the rest to --json.
func TestBannedAddressRowIsBounded(t *testing.T) {
	many := make([]string, 0, 40)
	for index := 0; index < 40; index++ {
		many = append(many, fmt.Sprintf("198.51.100.%d", index+1))
	}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy network fail2ban status", network.Fail2BanInfo{
		Installed: true, Running: true, CurrentlyBanned: 40, TotalBanned: 40, BannedIPs: many,
	})
	text := out.String()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if len(line) > 120 {
			t.Fatalf("a row ran to %d characters: %q", len(line), line)
		}
	}
	if !strings.Contains(text, "+35 more") {
		t.Fatalf("the addresses that were not shown are not accounted for:\n%s", text)
	}
	if strings.Contains(text, "198.51.100.40") {
		t.Fatalf("the row was not bounded:\n%s", text)
	}
}

// A firewall source such as "shadow-tls→snell" is more bytes than runes, and a
// port column padded by bytes would pull the source column after it left.
func TestColumnsAlignAroundNonASCIIValues(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy network firewall status", network.FirewallInfo{
		Available: true,
		Desired: []network.FirewallPortSpec{
			{Proto: "tcp", Port: 354, Sources: []string{"shadow-tls→snell"}},
			{Proto: "udp", Port: 28218, Sources: []string{"tuic"}},
		},
	})
	lines := strings.Split(out.String(), "\n")
	if first, second := strings.Index(lines[1], "shadow"), strings.Index(lines[2], "tuic"); first != second {
		t.Fatalf("source column is ragged (%d, %d):\n%s", first, second, out.String())
	}
}

// route test is one line: the name, the user's rule that takes it, where the
// traffic goes and whether the lookup leaves the same way.
func TestRouteTestIsOneLine(t *testing.T) {
	rule := application.RouteEntry{User: "alice", Label: "OpenAI/ChatGPT", Preset: "openai", Selector: "1", Outbound: "res-a"}
	fields := map[string]any{
		"evaluation": routing.Evaluation{Target: "chatgpt.com", Kind: "domain", Decision: routing.Decision{
			Rule: 3, MatchBy: "rule_set", Value: "geosite-openai", Outbound: "res-a", DNSServer: "res-a-dns", DNSVia: "res-a",
		}},
		"rule": rule, "address": "198.51.100.7:1080",
	}
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy route test", fields) {
		t.Fatal("route test produced no rendering")
	}
	if want := "chatgpt.com  1.OpenAI/ChatGPT (geosite-openai)  -> res-a: 198.51.100.7:1080  dns ok\n"; out.String() != want {
		t.Fatalf("got  %q\nwant %q", out.String(), want)
	}

	// Nothing took it: the route final, and the sets that could not be read.
	fields = map[string]any{"evaluation": routing.Evaluation{Target: "example.com", Kind: "domain",
		Decision:  routing.Decision{Rule: -1, MatchBy: "final", Outbound: "direct", DNSServer: "public4"},
		Unchecked: []string{"geosite-disney"}}}
	var final bytes.Buffer
	render(&final, palette{}, "gproxy route test", fields)
	want := "example.com  final  -> direct  dns ok\nunless an earlier rule set matches; not in sing-box's cache yet: geosite-disney\n"
	if final.String() != want {
		t.Fatalf("got  %q\nwant %q", final.String(), want)
	}

	// A resolver leaving by a different route than its traffic is the whole
	// reason to report it, so it must be visible rather than silent.
	fields = map[string]any{"evaluation": routing.Evaluation{Target: "chatgpt.com", Decision: routing.Decision{
		Rule: 3, MatchBy: "rule_set", Value: "geosite-openai", Outbound: "res-a", DNSServer: "public4",
	}}, "rule": rule}
	var leak bytes.Buffer
	render(&leak, palette{on: true}, "gproxy route test", fields)
	if !strings.Contains(leak.String(), ansiBad+"dns via direct") {
		t.Fatalf("a resolver leaving by another route was not flagged:\n%q", leak.String())
	}
}

// Installed nodes are listed where one is chosen for removal: the name, the
// port, the layer that encrypts it, and who it belongs to. `protocol list`
// answers what can be installed instead, so this is the listing that has to
// stay aligned and complete.
func TestInstalledNodeListingIsAlignedColumns(t *testing.T) {
	err := removeGuidance([]application.ProtocolNode{
		{Tag: "anytls_443", Type: "anytls", Port: 443, Security: "tls", Users: []string{"dhwang1"}},
		{Tag: "snell-v6", Type: "snell", Port: 1443, Security: "none", Users: []string{"dhwang1"},
			ShadowTLS: &application.ProtocolWrapper{Port: 8443}},
		{Tag: "tuic_21842", Type: "tuic", Port: 21842, Security: "tls", Users: []string{"dhwang2", "dhwang1"}},
	})
	var detail *application.Error
	if !errors.As(err, &detail) {
		t.Fatalf("removal guidance is not an application error: %v", err)
	}
	// The first hint line is the usage; the rows follow it.
	if !strings.HasPrefix(detail.Hint[0], "gproxy protocol remove ") {
		t.Fatalf("removal guidance does not lead with its command: %q", detail.Hint[0])
	}
	lines := detail.Hint[1:]
	text := strings.Join(lines, "\n")

	// A wrapped node is encrypted by its wrapper. The only port a listing
	// names is the node's own.
	if !strings.Contains(lines[1], "shadow-tls") {
		t.Fatalf("wrapped node does not name its layer: %q", lines[1])
	}
	if strings.Contains(text, "8443") {
		t.Fatalf("the wrapper's port outlived its removal:\n%s", text)
	}
	if !strings.Contains(lines[2], "dhwang2/dhwang1") {
		t.Fatalf("multiple owners were not listed: %q", lines[2])
	}
	// The port is named, since nothing else in the tree prints it for a human.
	for _, port := range []string{"443", "1443", "21842"} {
		if !strings.Contains(text, port) {
			t.Fatalf("listing does not name port %s:\n%s", port, text)
		}
	}

	// Every column starts at the same offset on every line. Anchored on the
	// user column, which is the one every row ends with, and on the port,
	// which is the first field after the padded node name.
	for name, offset := range map[string]func(string) int{
		"user": func(line string) int { return strings.Index(line, "dhwang") },
		"port": func(line string) int { return strings.IndexAny(line, "0123456789") + len("1.") },
	} {
		columns := map[int]bool{}
		for _, line := range lines {
			columns[offset(line)] = true
		}
		if len(columns) != 1 {
			t.Fatalf("the %s column starts at offsets %v, so it is ragged:\n%s", name, columns, text)
		}
	}
}

// protocol list answers what can be installed: the name and the forms it comes
// in, one terse row each.
func TestProtocolListIsTheInstallableCatalogue(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy protocol list", map[string]any{"protocols": catalogueData()}) {
		t.Fatal("catalogue produced no rendering")
	}
	want := []string{"1.vless(tls/reality)", "2.tuic", "3.anytls", "4.snell(v6/tls)"}
	got := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("catalogue is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The default export is read rather than pasted into a client, so it is
// grouped: a format at a time, a user at a time. It used to print a
// "user / tag (format)" header above every single link, which was more header
// than payload.
func TestSubscriptionGroupsByFormatThenUser(t *testing.T) {
	links := []application.SubscriptionLink{
		{Format: "surge", User: "alice", Tag: "snell-v6", Family: "v4", Content: "alice-snell-v4 = snell, 192.0.2.1, 8443"},
		{Format: "surge", User: "alice", Tag: "tuic_1", Family: "v4", Content: "alice-tuic-v4 = tuic-v5, 192.0.2.1, 1"},
		{Format: "surge", User: "bob", Tag: "tuic_1", Family: "v4", Content: "bob-tuic-v4 = tuic-v5, 192.0.2.1, 1"},
		{Format: "uri", User: "alice", Tag: "anytls_443", Family: "v4", Content: "anytls://x@192.0.2.1:443"},
	}
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy sub", map[string]any{"links": links}) {
		t.Fatal("subscription produced no rendering")
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{
		"[surge]", "alice",
		"alice-snell-v4 = snell, 192.0.2.1, 8443",
		"",
		"alice-tuic-v4 = tuic-v5, 192.0.2.1, 1",
		"",
		"bob", "bob-tuic-v4 = tuic-v5, 192.0.2.1, 1",
		"", "[uri]", "alice", "anytls://x@192.0.2.1:443",
	}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("subscription rendered\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	// A heading is printed once per group, not once per link.
	if strings.Count(out.String(), "alice\n") != 2 {
		t.Fatalf("a user heading repeated per link:\n%s", out.String())
	}

	// The two headings are coloured; the links are not, because they are what
	// gets copied.
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy sub", map[string]any{"links": links})
	for _, want := range []string{ansiSys + "[surge]", ansiCount + "alice"} {
		if !strings.Contains(coloured.String(), want) {
			t.Fatalf("heading %q is not coloured:\n%q", want, coloured.String())
		}
	}
	for _, line := range strings.Split(coloured.String(), "\n") {
		if strings.Contains(line, "snell,") && strings.Contains(line, "\x1b") {
			t.Fatalf("a link carries colour: %q", line)
		}
	}

	var empty bytes.Buffer
	render(&empty, palette{}, "gproxy sub", map[string]any{"links": []application.SubscriptionLink{}})
	if strings.TrimSpace(empty.String()) != "no links" {
		t.Fatalf("empty export rendered %q", empty.String())
	}
}

// The default view's mihomo section is a mihomo document: without the key and
// the sequence indent it read well and could not be loaded, which sent a reader
// back to re-export the same nodes with --mihomo.
func TestDefaultExportMihomoSectionIsLoadable(t *testing.T) {
	links := []application.SubscriptionLink{
		{Format: "mihomo", User: "alice", Tag: "vless_443", Family: "v4", Content: "{name: alice-vless-v4, type: vless}"},
		{Format: "mihomo", User: "bob", Tag: "vless_443", Family: "v4", Content: "{name: bob-vless-v4, type: vless}"},
		{Format: "uri", User: "alice", Tag: "vless_443", Family: "v4", Content: "vless://x@192.0.2.1:443"},
	}
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy sub", map[string]any{"links": links}) {
		t.Fatal("subscription produced no rendering")
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{
		"[mihomo]", "proxies:",
		"# alice", "  - {name: alice-vless-v4, type: vless}",
		"",
		"# bob", "  - {name: bob-vless-v4, type: vless}",
		"", "[uri]", "alice", "vless://x@192.0.2.1:443",
	}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("subscription rendered\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// Removing one member of a shared node leaves the node listening for everyone
// else. Saying "node removed" for that described the wrong outcome half the
// time, so each of the four cases has to read as itself.
func TestMembershipRemovalSaysWhatWent(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]any
		want   []string
		absent string
	}{
		{
			name:   "node kept for its other users",
			fields: map[string]any{"tag": "anytls_2083", "user": "bob", "removed": true, "node_removed": false},
			want:   []string{"anytls_2083", "kept", "bob", "removed"},
		},
		{
			name:   "last member takes the node",
			fields: map[string]any{"tag": "vless_reality_2053", "user": "alice", "removed": true, "node_removed": true},
			want:   []string{"vless_reality_2053", "removed", "last member"},
			absent: "kept",
		},
		{
			name:   "user was never a member",
			fields: map[string]any{"tag": "anytls_2083", "user": "carol", "removed": false, "node_removed": false},
			want:   []string{"anytls_2083", "kept", "carol", "not a member"},
		},
		{
			name:   "tag names nothing",
			fields: map[string]any{"tag": "anytls", "user": "carol", "removed": false},
			want:   []string{"anytls", "not found"},
			absent: "kept",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer
			if !render(&out, palette{}, "gproxy protocol remove", testCase.fields) {
				t.Fatal("no rendering")
			}
			for _, want := range testCase.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("rendering is missing %q:\n%s", want, out.String())
				}
			}
			if testCase.absent != "" && strings.Contains(out.String(), testCase.absent) {
				t.Fatalf("rendering says %q when it must not:\n%s", testCase.absent, out.String())
			}
		})
	}
}

func TestCommandHintColoursOnlyTheCommand(t *testing.T) {
	p := palette{on: true}
	cases := map[string]string{
		"gproxy protocol add snell --user <name>":      ansiCommand + "gproxy protocol add snell --user <name>" + ansiReset,
		"  run: gproxy user add alice":                 "  run: " + ansiCommand + "gproxy user add alice" + ansiReset,
		"  gproxy user list   lists the current names": "  " + ansiCommand + "gproxy user list" + ansiReset + "   lists the current names",
		"    --host  hostname or IP":                   "    --host  hostname or IP",
		"1.snell  --user <name>":                       "1.snell  --user <name>",
	}
	for line, want := range cases {
		if got := commandHint(p, line); got != want {
			t.Errorf("commandHint(%q) = %q, want %q", line, got, want)
		}
	}
	if got := commandHint(palette{}, "gproxy user list"); got != "gproxy user list" {
		t.Errorf("colour off still wrapped: %q", got)
	}
}

func TestUserListGroupsMembershipsUnderTheName(t *testing.T) {
	users := map[string]any{"users": []application.UserView{
		{Name: "alice", Memberships: []application.UserMembership{}},
		{Name: "bob", Routes: 2, Memberships: []application.UserMembership{
			{Tag: "anytls_443", Protocol: "anytls", Port: 443},
			{Tag: "snell-v6", Protocol: "snell-v6", Port: 1443, ShadowTLS: &application.ProtocolWrapper{Port: 8443, Version: 3}},
		}},
	}}
	var plain bytes.Buffer
	if !render(&plain, palette{}, "gproxy user list", users) {
		t.Fatal("user list produced no rendering")
	}
	want := "alice  no protocols\n" +
		"bob  2 routes\n" +
		"● anytls - 443\n" +
		"● snell-v6+shadow-tls-v3 - shadow-tls:8443 -> snell:1443\n"
	if plain.String() != want {
		t.Fatalf("plain rendering:\n%s\nwant:\n%s", plain.String(), want)
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy user list", users)
	for _, part := range []string{
		ansiUser + "bob" + ansiReset,
		ansiOK + "●" + ansiReset,
		ansiOK + "snell-v6+shadow-tls-v3" + ansiReset,
		"shadow-tls:" + ansiPort + "8443" + ansiReset,
		"snell:" + ansiBackend + "1443" + ansiReset,
		ansiPort + "443" + ansiReset,
	} {
		if !strings.Contains(coloured.String(), part) {
			t.Errorf("coloured rendering lacks %q:\n%q", part, coloured.String())
		}
	}
}

// config view prints the configuration alone, in shell-proxy's layout: keys in
// file order, each rule on one line, and the rule_set catalogue folded.
func TestConfigViewPrintsTheConfigurationInShellProxyLayout(t *testing.T) {
	raw := `{"log":{"level":"error"},"dns":{"servers":[{"tag":"public4","type":"https","server_port":443}],"rules":[{"action":"route","server":"public4","auth_user":["alice"]}],"strategy":"prefer_ipv4"},"route":{"final":"direct","rules":[{"action":"sniff"},{"ip_is_private":true,"outbound":"direct"}],"rule_set":[{"tag":"geosite-openai","url":"https://example.com"}]}}`
	var value json.RawMessage = json.RawMessage(raw)
	fields := map[string]any{"component": "sing-box", "configuration": value, "secrets_included": false}
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy config view", fields) {
		t.Fatal("config view produced no rendering")
	}
	want := `{
  "log": {
    "level": "error"
  },
  "dns": {
    "servers": [
      {
        "tag": "public4",
        "type": "https",
        "server_port": 443
      }
    ],
    "rules": [
      {"action":"route","server":"public4","auth_user":["alice"]}
    ],
    "strategy": "prefer_ipv4"
  },
  "route": {
    "final": "direct",
    "rules": [
      {"action":"sniff"},
      {"ip_is_private":true,"outbound":"direct"}
    ],
    "rule_set": [
      "..."
    ]
  }
}
`
	if out.String() != want {
		t.Fatalf("rendering:\n%s\nwant:\n%s", out.String(), want)
	}

	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy config view", fields)
	for _, part := range []string{jqKey + `"strategy"` + ansiReset, jqString + `"prefer_ipv4"` + ansiReset, jqPlain + "443" + ansiReset} {
		if !strings.Contains(coloured.String(), part) {
			t.Errorf("coloured rendering lacks %q", part)
		}
	}
}

// snell and shadow-tls print their configuration alone as well, with no
// rule folding and no HTML escaping of a redacted secret.
func TestConfigViewOmitsTheEnvelopeForEveryComponent(t *testing.T) {
	fields := map[string]any{"component": "snell", "configuration": map[string]any{"psk": "<redacted>"}, "secrets_included": false}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy config view", fields)
	if want := "{\n  \"psk\": \"<redacted>\"\n}\n"; out.String() != want {
		t.Fatalf("rendering = %q, want %q", out.String(), want)
	}
}

// Each status row has its own colour: system blue, network purple, protocol
// yellow, domain in the system amber. The service row keeps its state colours.
func TestStatusRowsHaveTheirOwnColours(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{on: true}, "gproxy status", statusFields())
	text := out.String()
	for _, want := range []string{
		ansiSystem + "Debian GNU/Linux 12 (bookworm)" + ansiReset,
		ansiNetwork + "2001:db8::1" + ansiReset,
		ansiProtocol + "alice" + ansiReset,
		ansiProtocol + "anytls" + ansiReset,
		ansiSys + "example.com" + ansiReset,
		ansiRunning + "sing-box" + ansiReset,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("status lacks %q:\n%q", want, text)
		}
	}
}

// add, modify and remove answer with the user's rules as they now stand, each
// pointing at its egress, in menu order; a removed rule keeps its place, struck
// through, or marked "(removed)" without colour.
func TestRuleChangesListTheUsersRules(t *testing.T) {
	rules := []application.RouteEntry{
		{User: "dhwang1", Index: 1, Label: "OpenAI/ChatGPT", Preset: "openai", Selector: "1", Outbound: "res2", Address: "198.51.100.7:12334"},
		{User: "dhwang1", Index: 2, Label: "WhatsApp", Preset: "whatsapp", Selector: "7", Outbound: "res1", Address: "198.51.100.8:12434"},
	}
	removed := []application.RouteEntry{
		{User: "dhwang1", Index: 3, Label: "Google", Preset: "google", Selector: "3", Outbound: "res1", Address: "198.51.100.8:12434"},
	}
	for _, command := range []string{"gproxy route rule add", "gproxy route rule modify"} {
		var out bytes.Buffer
		render(&out, palette{}, command, map[string]any{"user": "dhwang1", "affected_rules": 1, "rules": rules})
		want := "dhwang1:\n1.OpenAI/ChatGPT  -> res2: 198.51.100.7:12334\n7.WhatsApp        -> res1: 198.51.100.8:12434\n"
		if out.String() != want {
			t.Fatalf("%s rendered\n%s\nwant\n%s", command, out.String(), want)
		}
	}
	var plain bytes.Buffer
	render(&plain, palette{}, "gproxy route rule remove", map[string]any{"user": "dhwang1", "rules": rules, "removed": removed})
	want := "dhwang1:\n1.OpenAI/ChatGPT  -> res2: 198.51.100.7:12334\n3.Google          -> res1: 198.51.100.8:12434  (removed)\n7.WhatsApp        -> res1: 198.51.100.8:12434\n"
	if plain.String() != want {
		t.Fatalf("remove rendered\n%s\nwant\n%s", plain.String(), want)
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy route rule remove", map[string]any{"user": "dhwang1", "rules": rules, "removed": removed})
	text := coloured.String()
	if !strings.Contains(text, ansiRemoved+"Google"+ansiReset) || !strings.Contains(text, ansiRemoved+"-> res1: 198.51.100.8:12434"+ansiReset) {
		t.Fatalf("the removed rule is not struck through: %q", text)
	}
	if strings.Contains(text, "(removed)") {
		t.Fatalf("colour output carries the plain marker: %q", text)
	}
	// Struck-through marks take no extra columns.
	stripped := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(text, "")
	if strings.ReplaceAll(stripped, "  (removed)", "") != strings.ReplaceAll(plain.String(), "  (removed)", "") {
		t.Fatalf("strike changed the layout:\n%s", stripped)
	}
}

// The rule list reads like shell-proxy's: a chain as "tag: host:port", direct
// on its own, each preset coloured by kind, and row 10 aligned with row 9.
func TestRuleListColoursAndAlignsLikeShellProxy(t *testing.T) {
	entries := []application.RouteEntry{}
	presets := []string{"openai", "google", "twitter", "ads", "anthropic", "github", "discord", "xai", "meta", "ai-intl"}
	for index, name := range presets {
		preset, _ := routing.FindPreset(name)
		entries = append(entries, application.RouteEntry{User: "alice", Index: index + 1, Label: preset.Label, Preset: name, Outbound: "res1", Address: "198.51.100.7:1080"})
	}
	entries = append(entries, application.RouteEntry{User: "bob", Index: 1, Label: "Netflix", Preset: "netflix", Outbound: "direct"})
	fields := map[string]any{"rules": entries}

	var plain bytes.Buffer
	render(&plain, palette{}, "gproxy route rule list", fields)
	text := plain.String()
	for _, want := range []string{"-> res1: 198.51.100.7:1080", "-> direct", "10.AI (Intl)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rule list lacks %q:\n%s", want, text)
		}
	}
	// The arrow sits in one column whatever the row number's width.
	column := -1
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "-> res1") {
			continue
		}
		if at := strings.Index(line, "->"); column == -1 {
			column = at
		} else if at != column {
			t.Fatalf("arrow moved from column %d to %d:\n%s", column, at, text)
		}
	}

	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy route rule list", fields)
	for _, want := range []string{
		ansiPresetAI + "OpenAI/ChatGPT" + ansiReset,
		ansiPresetPlatform + "Google" + ansiReset,
		ansiPresetSocial + "Twitter/X" + ansiReset,
		ansiPresetBlock + "Ad blocking" + ansiReset,
		ansiChainTag + "res1" + ansiReset + ": " + ansiChainAddress + "198.51.100.7:1080" + ansiReset,
		ansiDirect + "direct" + ansiReset,
		ansiUser + "alice" + ansiReset,
	} {
		if !strings.Contains(coloured.String(), want) {
			t.Errorf("coloured rule list lacks %q", want)
		}
	}
	// Colour codes take no columns: the coloured arrows line up as the plain ones do.
	stripped := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(coloured.String(), "")
	if stripped != text {
		t.Fatalf("colour changed the layout:\n%s\nplain:\n%s", stripped, text)
	}
}

// The rule listing numbers each rule with its preset menu index, which is what
// --rules takes, and keeps the arrow column aligned across one- and
// two-character marks.
func TestRuleListNumbersRulesByPresetIndex(t *testing.T) {
	entries := []application.RouteEntry{
		{User: "alice", Index: 1, Label: "OpenAI/ChatGPT", Preset: "openai", Selector: "1", Outbound: "direct"},
		{User: "alice", Index: 2, Label: "xAI/Grok", Preset: "xai", Selector: "j", Outbound: "direct"},
		{User: "alice", Index: 3, Label: "example.com", Selector: "c1", Outbound: "direct"},
	}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy route rule list", map[string]any{"rules": entries})
	text := out.String()
	for _, want := range []string{"1.OpenAI/ChatGPT", "j.xAI/Grok", "c1.example.com"} {
		if !strings.Contains(text, want) {
			t.Fatalf("listing lacks %q:\n%s", want, text)
		}
	}
	column := -1
	for _, line := range strings.Split(strings.TrimSpace(text), "\n")[1:] {
		at := strings.Index(line, "->")
		if column == -1 {
			column = at
		} else if at != column {
			t.Fatalf("arrow column moved:\n%s", text)
		}
	}
}

// A logger's padding is collapsed for display; indentation is kept.
func TestLogLinesCollapseRepeatedSpaces(t *testing.T) {
	for line, want := range map[string]string{
		"2026-09-23T01:02:31.373530Z  INFO shadow_tls: Start 2-thread Server with:": "2026-09-23T01:02:31.373530Z INFO shadow_tls: Start 2-thread Server with:",
		"    at main.go:12   (inlined)":                                             "    at main.go:12 (inlined)",
		"plain line":                                                                "plain line",
	} {
		if got := logLine(palette{}, line); got != want {
			t.Errorf("logLine(%q) = %q, want %q", line, got, want)
		}
	}
}

// A removal answers the way `user list` reads: each user and the membership
// that went, struck through with its colours kept, or marked "(removed)".
func TestProtocolRemovalListsTheMembershipsThatWent(t *testing.T) {
	fields := map[string]any{"tag": "anytls_443", "user": "dhwang2", "removed": true, "node_removed": false,
		"removed_memberships": []application.UserView{
			{Name: "dhwang2", Memberships: []application.UserMembership{{Tag: "anytls_443", Protocol: "anytls", Port: 443}}},
		}}
	var plain bytes.Buffer
	render(&plain, palette{}, "gproxy protocol remove", fields)
	if want := "dhwang2: anytls - 443 (removed)\n"; plain.String() != want {
		t.Fatalf("plain = %q, want %q", plain.String(), want)
	}

	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy protocol remove", fields)
	text := strings.TrimRight(coloured.String(), "\n")
	for _, part := range []string{ansiUser + "dhwang2", ansiOK + "anytls", ansiPort + "443"} {
		if !strings.Contains(text, part) {
			t.Errorf("coloured removal lacks %q: %q", part, text)
		}
	}
	// The strike opens the line and is taken up again after every reset, so
	// no piece of it, separators included, is left unstruck.
	if !strings.HasPrefix(text, "\x1b[9m") || strings.Count(text, ansiReset) != strings.Count(text, ansiReset+"\x1b[9m")+1 {
		t.Fatalf("the strike does not cover the whole line: %q", text)
	}

	// A whole node lists every owner; a wrapped snell shows its path.
	whole := map[string]any{"tag": "snell-v6", "removed": true, "removed_memberships": []application.UserView{
		{Name: "alice", Memberships: []application.UserMembership{{Tag: "snell-v6", Protocol: "snell-v6", Port: 1443, ShadowTLS: &application.ProtocolWrapper{Port: 8443, Version: 3}}}},
		{Name: "bob", Memberships: []application.UserMembership{{Tag: "snell-v6", Protocol: "snell-v6", Port: 1443, ShadowTLS: &application.ProtocolWrapper{Port: 8443, Version: 3}}}},
	}}
	var node bytes.Buffer
	render(&node, palette{}, "gproxy protocol remove", whole)
	want := "alice: snell-v6+shadow-tls-v3 - shadow-tls:8443 -> snell:1443 (removed)\nbob: snell-v6+shadow-tls-v3 - shadow-tls:8443 -> snell:1443 (removed)\n"
	if node.String() != want {
		t.Fatalf("node removal = %q, want %q", node.String(), want)
	}
}

// Removing a membership a user does not have changes nothing, and says so in
// the same user-list line, unstruck, with the reason.
func TestProtocolRemovalSaysWhyNothingWent(t *testing.T) {
	membership := application.UserMembership{Tag: "anytls_443", Protocol: "anytls", Port: 443}
	for exists, want := range map[bool]string{
		true:  "dhwang1: anytls - 443 (not a member)\n",
		false: "dhwang1: anytls - 443 (no such user)\n",
	} {
		fields := map[string]any{"tag": "anytls_443", "user": "dhwang1", "removed": false, "node_removed": false, "membership": membership, "user_exists": exists}
		var plain bytes.Buffer
		render(&plain, palette{}, "gproxy protocol remove", fields)
		if plain.String() != want {
			t.Fatalf("plain = %q, want %q", plain.String(), want)
		}
		var coloured bytes.Buffer
		render(&coloured, palette{on: true}, "gproxy protocol remove", fields)
		text := coloured.String()
		if strings.Contains(text, "\x1b[9m") {
			t.Fatalf("an unremoved membership was struck: %q", text)
		}
		for _, part := range []string{ansiUser + "dhwang1", ansiOK + "anytls", ansiPort + "443", ansiHint + "("} {
			if !strings.Contains(text, part) {
				t.Errorf("coloured line lacks %q: %q", part, text)
			}
		}
	}
}

// An install answers with the enrolled user's new membership as `user list`
// shows it, and says whether it was added or already there.
func TestProtocolAddShowsTheNewMembership(t *testing.T) {
	for alreadyMember, want := range map[bool]string{
		false: "dhwang0: anytls - 443 (added)\n",
		true:  "dhwang0: anytls - 443 (already a member)\n",
	} {
		node := application.ProtocolNode{Tag: "anytls_443", Type: "anytls", Port: 443, Security: "tls", Users: []string{"dhwang0"}, Added: "dhwang0", AlreadyMember: alreadyMember}
		var plain bytes.Buffer
		render(&plain, palette{}, "gproxy protocol add", node)
		if plain.String() != want {
			t.Fatalf("plain = %q, want %q", plain.String(), want)
		}
	}
	snell := application.ProtocolNode{Tag: "snell-v6", Type: "snell", Port: 1443, Users: []string{"dhwang2"}, Added: "dhwang2",
		ShadowTLS: &application.ProtocolWrapper{Port: 354, Version: 3}}
	var out bytes.Buffer
	render(&out, palette{on: true}, "gproxy protocol add", snell)
	text := out.String()
	for _, part := range []string{ansiUser + "dhwang2", ansiOK + "snell-v6+shadow-tls-v3", ansiPort + "354", ansiBackend + "1443", ansiHint + "(added)"} {
		if !strings.Contains(text, part) {
			t.Errorf("coloured install lacks %q: %q", part, text)
		}
	}
}

// A rename is one line, old name to new; renaming to the same name says it
// changed nothing.
func TestUserRenameIsOneLine(t *testing.T) {
	for fields, want := range map[*map[string]any]string{
		{"user": "dhwang8", "previous": "dhwang0", "renamed": true}:  "dhwang0 -> dhwang8 (changed)\n",
		{"user": "dhwang0", "previous": "dhwang0", "renamed": false}: "dhwang0 (unchanged)\n",
	} {
		var out bytes.Buffer
		render(&out, palette{}, "gproxy user rename", *fields)
		if out.String() != want {
			t.Fatalf("rename = %q, want %q", out.String(), want)
		}
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy user rename", map[string]any{"user": "dhwang8", "previous": "dhwang0", "renamed": true})
	if want := ansiUser + "dhwang0" + ansiReset + " -> " + ansiUser + "dhwang8" + ansiReset + " " + ansiHint + "(changed)" + ansiReset + "\n"; coloured.String() != want {
		t.Fatalf("coloured rename = %q, want %q", coloured.String(), want)
	}
}

// A preset an add found already there keeps its line, struck through with the
// reason; the rest of the list reads as usual.
func TestRuleAddMarksWhatWasAlreadyAdded(t *testing.T) {
	rules := []application.RouteEntry{
		{User: "dhwang6", Label: "OpenAI/ChatGPT", Preset: "openai", Selector: "1", Outbound: "res2", Address: "198.51.100.7:12334"},
		{User: "dhwang6", Label: "Netflix", Preset: "netflix", Selector: "b", Outbound: "res1", Address: "198.51.100.8:12434"},
	}
	fields := map[string]any{"user": "dhwang6", "presets": []string{"netflix"}, "already_added": []string{"openai"}, "rules": rules}
	var plain bytes.Buffer
	render(&plain, palette{}, "gproxy route rule add", fields)
	want := "dhwang6:\n1.OpenAI/ChatGPT  -> res2: 198.51.100.7:12334  (already added)\nb.Netflix         -> res1: 198.51.100.8:12434\n"
	if plain.String() != want {
		t.Fatalf("plain =\n%s\nwant\n%s", plain.String(), want)
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy route rule add", fields)
	text := coloured.String()
	if !strings.Contains(text, ansiRemoved+"OpenAI/ChatGPT"+ansiReset) || !strings.Contains(text, ansiHint+"(already added)"+ansiReset) {
		t.Fatalf("the existing rule is not struck with its reason: %q", text)
	}
	if strings.Contains(text, ansiRemoved+"Netflix") {
		t.Fatalf("the added rule was struck: %q", text)
	}
}

// bbr status is one line: enabled already means the congestion control is
// bbr, so the algorithm is named only when it is something else.
func TestBBRIsOneLine(t *testing.T) {
	for _, c := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"current": "bbr", "enabled": true}, "bbr enabled\n"},
		{map[string]any{"current": "cubic", "enabled": false}, "bbr not enabled (using cubic)\n"},
	} {
		for _, command := range []string{"gproxy network bbr status", "gproxy network bbr enable"} {
			var out bytes.Buffer
			if !render(&out, palette{}, command, c.fields) || out.String() != c.want {
				t.Fatalf("%s: got %q, want %q", command, out.String(), c.want)
			}
		}
	}
}

// With the list written to its file, the row is the path: a busy host bans
// hundreds, and the file holds them one per line.
func TestBannedAddressesArePointedAtTheirFile(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy network fail2ban status", network.Fail2BanInfo{
		Installed: true, Running: true, CurrentlyBanned: 2, TotalBanned: 9,
		BannedIPs: []string{"198.51.100.5", "198.51.100.9"}, BannedFile: "/etc/go-proxy/logs/fail2ban-banned.txt",
	})
	text := out.String()
	if !strings.Contains(text, "banned ip     /etc/go-proxy/logs/fail2ban-banned.txt\n") || strings.Contains(text, "198.51.100.5") {
		t.Fatalf("the banned row is not the file path:\n%s", text)
	}
	// Nothing banned: no row, whether or not the file was written.
	var none bytes.Buffer
	render(&none, palette{}, "gproxy network fail2ban status", network.Fail2BanInfo{
		Installed: true, Running: true, BannedIPs: []string{}, BannedFile: "/etc/go-proxy/logs/fail2ban-banned.txt",
	})
	if strings.Contains(none.String(), "banned ip") {
		t.Fatalf("an empty list printed a row:\n%s", none.String())
	}
}

// status names what keeps the ssh jail on. disable stops fail2ban itself and
// says how many bans went with it; enable says whether it started the service.
func TestFail2banChangesAreOneLine(t *testing.T) {
	managed := network.Fail2BanInfo{Installed: true, Running: true, SSHJailEnabled: true, Managed: true,
		JailSources: []string{"jail.d/defaults-debian.conf", "jail.d/sshd.local"}}
	var status bytes.Buffer
	render(&status, palette{}, "gproxy network fail2ban status", managed)
	if !strings.Contains(status.String(), "ssh jail      on  (gproxy, jail.d/defaults-debian.conf, jail.d/sshd.local)\n") {
		t.Fatalf("status:\n%s", status.String())
	}
	for _, c := range []struct {
		info network.Fail2BanInfo
		want string
	}{
		{network.Fail2BanInfo{Installed: true, Change: "stopped", BansLifted: 17}, "fail2ban  stopped (service disabled; 17 bans lifted)\n"},
		{network.Fail2BanInfo{Installed: true, Change: "stopped"}, "fail2ban  stopped (service disabled)\n"},
		{network.Fail2BanInfo{Installed: true, Change: "already stopped"}, "fail2ban  already stopped\n"},
		{network.Fail2BanInfo{Installed: true, Running: true, Change: "started"}, "fail2ban  running (started with gproxy's ssh jail)\n"},
		{network.Fail2BanInfo{Installed: true, Running: true, Change: "added"}, "fail2ban  running (gproxy ssh jail added)\n"},
		{network.Fail2BanInfo{Installed: true, Running: true, Change: "already managed"}, "fail2ban  running (gproxy ssh jail already in place)\n"},
	} {
		var out bytes.Buffer
		if render(&out, palette{}, "gproxy network fail2ban disable", c.info); out.String() != c.want {
			t.Fatalf("%s: got %q, want %q", c.info.Change, out.String(), c.want)
		}
	}
	// Stopped: the service and the jail, nothing else.
	var stopped bytes.Buffer
	render(&stopped, palette{}, "gproxy network fail2ban status", network.Fail2BanInfo{Installed: true,
		JailSources: []string{"jail.d/sshd.local"}})
	if stopped.String() != "1.fail2ban  stopped\n2.ssh jail  off\n" {
		t.Fatalf("stopped status = %q", stopped.String())
	}
}

// user add and remove are one line each, like rename: the name and what
// happened, with what a removal took along counted beside it.
func TestUserChangesAreOneLine(t *testing.T) {
	for _, c := range []struct {
		command string
		fields  map[string]any
		want    string
	}{
		{"gproxy user add", map[string]any{"user": "alice", "added": true}, "alice (added)\n"},
		{"gproxy user add", map[string]any{"user": "alice", "added": false}, "alice (already exists)\n"},
		{"gproxy user remove", map[string]any{"user": "bob", "removed": true, "nodes_removed": 1, "rules_removed": 3}, "bob (removed; 1 node, 3 rules)\n"},
		{"gproxy user remove", map[string]any{"user": "bob", "removed": true, "nodes_removed": 0, "rules_removed": 0}, "bob (removed)\n"},
		{"gproxy user remove", map[string]any{"user": "ghost", "removed": false}, "ghost (not found)\n"},
	} {
		var out bytes.Buffer
		if !render(&out, palette{}, c.command, c.fields) || out.String() != c.want {
			t.Fatalf("%s %v: got %q, want %q", c.command, c.fields, out.String(), c.want)
		}
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy user remove", map[string]any{"user": "bob", "removed": true, "rules_removed": 2})
	if !strings.HasPrefix(coloured.String(), "\x1b[9m") || !strings.Contains(coloured.String(), "(removed; 2 rules)") {
		t.Fatalf("a removed user is not struck through: %q", coloured.String())
	}
}

// update answers in one line: the running version, the release it would move
// to when there is one, and whether it moved.
func TestSelfUpdateIsOneLine(t *testing.T) {
	for _, c := range []struct {
		check update.SelfUpdateCheck
		want  string
	}{
		{update.SelfUpdateCheck{CurrentVersion: "v0.3.1", LatestVersion: "v0.3.1"}, "go-proxy-cli version: v0.3.1 (already latest version)\n"},
		{update.SelfUpdateCheck{CurrentVersion: "0.3.0", LatestVersion: "v0.3.1", UpdateAvail: true}, "go-proxy-cli version: v0.3.0 -> v0.3.1 (updates available)\n"},
		{update.SelfUpdateCheck{CurrentVersion: "v0.3.0", LatestVersion: "v0.3.1", UpdateAvail: true, Updated: true}, "go-proxy-cli version: v0.3.0 -> v0.3.1 (updated)\n"},
		{update.SelfUpdateCheck{CurrentVersion: "v0.3.1", LatestVersion: "v0.3.0", UpdateAvail: true}, "go-proxy-cli version: v0.3.1 -> v0.3.0 (downgrade available)\n"},
		{update.SelfUpdateCheck{CurrentVersion: "dev", LatestVersion: "v0.3.1"}, "go-proxy-cli version: dev (development build; latest v0.3.1)\n"},
	} {
		check := c.check
		var out bytes.Buffer
		if !render(&out, palette{}, "gproxy update", &check) || out.String() != c.want {
			t.Fatalf("got %q, want %q", out.String(), c.want)
		}
	}
}

// cert status and ensure answer in one line: the domain and its state.
func TestCertificateIsOneLine(t *testing.T) {
	expires := time.Now().Add(88*24*time.Hour + time.Hour)
	for _, c := range []struct {
		data any
		want string
	}{
		{cert.Status{Domain: "proxy.example.com", Ready: true, ExpiresAt: &expires}, "domain: proxy.example.com (expires in 88 days)\n"},
		{cert.Status{Domain: "proxy.example.com"}, "domain: proxy.example.com (not issued)\n"},
		{cert.Status{}, "domain: none configured\n"},
	} {
		for _, command := range []string{"gproxy cert status", "gproxy cert ensure"} {
			var out bytes.Buffer
			if !render(&out, palette{}, command, c.data) || out.String() != c.want {
				t.Fatalf("%s: got %q, want %q", command, out.String(), c.want)
			}
		}
	}
}

// uninstall --preview lists each path once, grouped by folder with one colour
// per folder; the firewall table and bashrc block say what they are.
func TestUninstallPreviewGroupsByFolder(t *testing.T) {
	fields := map[string]any{
		"paths": []string{
			"/etc/systemd/system/sing-box.service", "/etc/go-proxy", "/etc/systemd/system/caddy-sub.service", "/usr/bin/gproxy",
		},
		"bashrc_block":   "/etc/bash.bashrc",
		"firewall_table": "inet proxy_firewall",
	}
	var plain bytes.Buffer
	render(&plain, palette{}, "gproxy uninstall", fields)
	want := "/etc/systemd/system/sing-box.service\n/etc/systemd/system/caddy-sub.service\n/etc/go-proxy\n/usr/bin/gproxy\n" +
		"/etc/bash.bashrc(go-proxy completion block)\ninet proxy_firewall(nftables table)\n"
	if plain.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", plain.String(), want)
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy uninstall", fields)
	lines := strings.Split(coloured.String(), "\n")
	colour := func(line string) string { return line[:strings.Index(line, "m")+1] }
	if colour(lines[0]) != colour(lines[1]) || colour(lines[1]) == colour(lines[2]) || colour(lines[2]) == colour(lines[3]) {
		t.Fatalf("folders are not told apart by colour: %q", coloured.String())
	}
	var empty bytes.Buffer
	render(&empty, palette{}, "gproxy uninstall", map[string]any{"paths": []string{}})
	if empty.String() != "nothing to remove\n" {
		t.Fatalf("empty = %q", empty.String())
	}
}

// core check puts the version first and what it means attached in brackets,
// one numbered row per core with the names aligned.
func TestCoreCheckAttachesTheNote(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy core check", []*core.UpdateCheck{
		{Component: core.CompSingBox, Installed: true, CurrentVersion: "1.14.1", LatestVersion: "v1.14.2", UpdateAvail: true},
		{Component: core.CompSnell, Installed: true, CurrentVersion: "6.0.0rc2", LatestVersion: "6.0.0rc2"},
		{Component: core.CompShadowTLS, Installed: true, CurrentVersion: "", LatestVersion: "0.2.25"},
		{Component: core.CompCaddy, LatestVersion: "v2.11.4"},
	})
	want := "1.sing-box    1.14.1 -> v1.14.2(update available)\n" +
		"2.snell       6.0.0rc2(up to date)\n" +
		"3.shadow-tls  version unknown(latest 0.2.25)\n" +
		"4.caddy       not installed(latest v2.11.4)\n"
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}
}
