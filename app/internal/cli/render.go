package cli

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/mod/semver"

	"go-proxy/internal/application"
	"go-proxy/internal/cert"
	"go-proxy/internal/core"
	"go-proxy/internal/network"
	"go-proxy/internal/routing"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
	"go-proxy/internal/subscription"
	"go-proxy/internal/update"
	"go-proxy/pkg/textutil"
)

// Truecolor SGR codes matching the v0.1.59 interface palette, so the CLI reads
// the way the terminal interface did. Colour is opt-out rather than negotiated:
// a terminal that cannot render it is excluded by the device check below, not
// by degrading to a smaller palette.
const (
	ansiReset = "\x1b[0m"
	ansiOK    = "\x1b[38;2;152;195;121m" // running, ready
	ansiBad   = "\x1b[38;2;224;108;117m" // stopped, failing
	// Darker variants for the service row, where many names sit side by side and
	// the brighter pair above reads as alarming rather than informative.
	ansiRunning = "\x1b[38;2;34;120;54m"   // running
	ansiStopped = "\x1b[38;2;168;42;42m"   // installed but not working
	ansiUnknown = "\x1b[38;2;110;110;110m" // any other state, including absent
	ansiSys     = "\x1b[38;2;245;158;11m"  // system values
	ansiCount   = "\x1b[38;2;97;175;239m"  // counts
	ansiLabel   = "\x1b[38;2;229;231;235m" // row labels
	ansiHint    = "\x1b[38;2;107;114;128m" // secondary detail
	ansiNote    = "\x1b[38;2;156;163;175m" // a bracketed note, brackets included
	ansiCommand = "\x1b[38;2;86;182;194m"  // a command to type
	// One colour per status row, so the rows are told apart at a glance.
	// The domain row keeps ansiSys.
	ansiSystem   = "\x1b[38;2;97;175;239m"  // blue
	ansiNetwork  = "\x1b[38;2;198;120;221m" // purple
	ansiProtocol = "\x1b[38;2;229;192;123m" // yellow
	// The user list keeps the v0.1.59 overview's colours: a bold name, the
	// port a client connects to in purple, and the port a wrapper forwards to
	// in cyan.
	ansiUser    = "\x1b[1;38;2;97;175;239m"
	ansiPort    = "\x1b[38;2;198;120;221m"
	ansiBackend = "\x1b[38;2;86;182;194m"
	// The rule list keeps shell-proxy's colours, which are the terminal's
	// own bold palette rather than truecolor: a preset by what kind of
	// service it is, a chain tag in cyan and its address in yellow, direct
	// in green.
	ansiPresetAI       = "\x1b[35;1m"
	ansiPresetPlatform = "\x1b[34;1m"
	ansiPresetSocial   = "\x1b[33;1m"
	ansiPresetBlock    = "\x1b[31;1m"
	ansiPresetCustom   = "\x1b[37;1m"
	ansiChainTag       = "\x1b[36;1m"
	ansiChainAddress   = "\x1b[33;1m"
	ansiDirect         = "\x1b[32;1m"
	// A removed rule: struck through and dimmed, one style for the whole
	// line, because a colour's reset would also end the strike.
	ansiRemoved = "\x1b[9;38;2;110;110;110m"
)

// presetColour is shell-proxy's routing_preset_label_colored: AI services,
// platforms, social networks, and blocking each have a colour.
func presetColour(name string) string {
	switch name {
	case "openai", "anthropic", "xai", "ai-intl":
		return ansiPresetAI
	case "google", "youtube", "github", "netflix", "disney", "mytvsuper", "spotify", "tiktok", "microsoft", "paypal":
		return ansiPresetPlatform
	case "telegram", "twitter", "whatsapp", "facebook", "discord", "instagram", "reddit", "linkedin", "meta", "messenger":
		return ansiPresetSocial
	case "ads":
		return ansiPresetBlock
	}
	return ansiPresetCustom
}

type palette struct{ on bool }

func (p palette) wrap(code, text string) string {
	if !p.on || text == "" {
		return text
	}
	return code + text + ansiReset
}

func (p palette) ok(text string) string       { return p.wrap(ansiOK, text) }
func (p palette) bad(text string) string      { return p.wrap(ansiBad, text) }
func (p palette) sys(text string) string      { return p.wrap(ansiSys, text) }
func (p palette) count(text string) string    { return p.wrap(ansiCount, text) }
func (p palette) label(text string) string    { return p.wrap(ansiLabel, text) }
func (p palette) hint(text string) string     { return p.wrap(ansiHint, text) }
func (p palette) running(text string) string  { return p.wrap(ansiRunning, text) }
func (p palette) stopped(text string) string  { return p.wrap(ansiStopped, text) }
func (p palette) unknown(text string) string  { return p.wrap(ansiUnknown, text) }
func (p palette) command(text string) string  { return p.wrap(ansiCommand, text) }
func (p palette) user(text string) string     { return p.wrap(ansiUser, text) }
func (p palette) port(text string) string     { return p.wrap(ansiPort, text) }
func (p palette) backend(text string) string  { return p.wrap(ansiBackend, text) }
func (p palette) system(text string) string   { return p.wrap(ansiSystem, text) }
func (p palette) network(text string) string  { return p.wrap(ansiNetwork, text) }
func (p palette) protocol(text string) string { return p.wrap(ansiProtocol, text) }

// commandHint colours the part of a guidance line a reader types: a line that
// is a gproxy command, or the command after "run: ". An explanation set off
// after it by three spaces stays plain, as do lists and prose.
func commandHint(p palette, line string) string {
	body := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(body)]
	prefix := ""
	if rest, ok := strings.CutPrefix(body, "run: "); ok {
		prefix, body = "run: ", rest
	}
	if !strings.HasPrefix(body, "gproxy ") {
		return line
	}
	// A trailing "(note)" is said about the command, not typed with it.
	remarkText := ""
	if strings.HasSuffix(body, ")") {
		if index := strings.LastIndex(body, " ("); index >= 0 {
			body, remarkText = body[:index], body[index+1:]
		}
	}
	if remarkText != "" {
		return indent + prefix + p.command(body) + " " + remark(p, remarkText)
	}
	// An explanation without brackets follows the command after three spaces.
	explanation := ""
	if index := strings.Index(body, "   "); index >= 0 {
		body, explanation = body[:index], body[index:]
	}
	return indent + prefix + p.command(body) + explanation
}

// colorEnabled reports whether to emit escape sequences. Anything that is not a
// character device gets none, which covers pipes, files and captured output, so
// a caller parsing the human rendering never has to strip them.
func colorEnabled(w io.Writer, disabled bool) bool {
	if disabled || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// row is one line of rendered output: an identity and its detail.
type row struct{ label, value string }

// section is a run of rows under a heading, used where the rows belong to
// something. Numbering restarts inside each section because that is what the
// commands acting on those rows expect: `route rule remove --rules` indexes
// per user, so one continuous numbering would print indexes it rejects.
type section struct {
	title string
	rows  []row
	// marks, when set, replace the row numbers: the rule listing prints the
	// preset menu index each rule is selected by.
	marks []string
}

// mark is what a row is numbered with: its own mark, or its position.
func (s section) mark(number int) string {
	if number < len(s.marks) {
		return s.marks[number]
	}
	return strconv.Itoa(number + 1)
}

// writeRows is the single layout every rendered command uses: numbered from one,
// labels left-aligned with no indent, detail in a second column. Having one
// implementation is what makes the output of one command read like another's.
func writeRows(w io.Writer, p palette, rows []row) bool {
	return writeSections(w, p, []section{{rows: rows}})
}

// writeAligned is writeRows without the row numbers: labels in the label
// colour, padded so the detail column lines up. For listings whose rows are
// named rather than picked by number, such as the proxy cores.
func writeAligned(w io.Writer, p palette, rows []row) bool {
	width := 0
	for _, r := range rows {
		width = max(width, displayWidth(r.label))
	}
	for _, r := range rows {
		line := p.label(r.label)
		if r.value != "" {
			line = p.label(pad(r.label, width)) + "  " + r.value
		}
		fmt.Fprintln(w, line)
	}
	return true
}

// writeSections is that same layout with headings. The label column is sized
// across every section so the detail column stays in one place down the whole
// output rather than stepping in and out per group.
func writeSections(w io.Writer, p palette, sections []section) bool {
	// The width counts the row number with the label, so row 10 lines up
	// with row 9: padding the label alone pushed every two-digit row's
	// detail one column right.
	width := 0
	for _, s := range sections {
		for number, r := range s.rows {
			if size := displayWidth(s.mark(number)) + displayWidth(r.label); size > width {
				width = size
			}
		}
	}
	for index, s := range sections {
		if s.title != "" {
			if index > 0 {
				if _, err := fmt.Fprintln(w); err != nil {
					return true
				}
			}
			if _, err := fmt.Fprintln(w, s.title); err != nil {
				return true
			}
		}
		for number, r := range s.rows {
			// A row with no detail is just the label: padding it would leave
			// trailing whitespace, which shows up in diffs and copied output.
			mark := s.mark(number)
			line := fmt.Sprintf("%s.%s", mark, p.label(r.label))
			if r.value != "" {
				line = fmt.Sprintf("%s.%s  %s", mark, p.label(pad(r.label, width-displayWidth(mark))), r.value)
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				return true
			}
		}
	}
	return true
}

// serviceName shortens a unit name for display. The suffixes carry no meaning
// for a reader choosing which service to restart.
func serviceName(unit string) string {
	switch unit {
	case "snell-v6":
		return "snell"
	case "caddy-sub":
		return "caddy"
	case "proxy-watchdog":
		return "watchdog"
	}
	return unit
}

// serviceState reduces a unit to the only distinction a reader acts on: it is
// running, or it is not. Whether a stopped unit is installed at all is carried
// by colour rather than by another word.
func serviceState(p palette, state service.Status) string {
	switch {
	case state.Running:
		return p.running("running")
	case !state.Installed:
		return p.unknown("stopped")
	default:
		return p.stopped("stopped")
	}
}

// render writes a human rendering for command and reports whether it produced
// one. A command without a rendering falls back to the indented JSON the caller
// already emits, so adding renderings is incremental.
func render(w io.Writer, p palette, command string, data any) bool {
	// Not every result is a map: core returns a slice and fail2ban a struct, so
	// the raw value is passed through and each renderer asserts its own shape.
	fields, _ := data.(map[string]any)
	if fields == nil {
		switch command {
		case "gproxy core version", "gproxy core check", "gproxy network fail2ban status", "gproxy network firewall status",
			"gproxy network firewall apply", "gproxy network fail2ban enable", "gproxy network fail2ban disable",
			"gproxy server start", "gproxy server stop", "gproxy server restart",
			"gproxy cert status", "gproxy cert ensure", "gproxy update",
			"gproxy protocol add", "gproxy route chain add", "gproxy route chain modify":
		default:
			return false
		}
	}
	switch command {
	case "gproxy status":
		return renderStatus(w, p, fields)
	case "gproxy server status", "gproxy server start", "gproxy server stop", "gproxy server restart":
		return renderServiceTable(w, p, data)
	case "gproxy user list":
		return renderUsers(w, p, fields)
	case "gproxy protocol list":
		return renderCatalogue(w, p, fields)
	case "gproxy core version", "gproxy core check":
		return renderCores(w, p, data)
	case "gproxy route rule list":
		return renderRules(w, p, fields)
	case "gproxy route chain list":
		return renderChains(w, p, fields)
	case "gproxy sub":
		return renderSubscription(w, p, fields)
	case "gproxy route test":
		return renderRouteTest(w, p, fields)
	case "gproxy route direct list", "gproxy route direct set", "gproxy route sync-dns":
		return renderDirect(w, p, fields)
	case "gproxy log":
		return renderLog(w, p, fields)
	case "gproxy config validate":
		return renderValidation(w, p, fields)
	case "gproxy config view":
		return renderConfig(w, p, fields)
	case "gproxy route final list", "gproxy route final set":
		return renderFinal(w, p, fields)
	case "gproxy network firewall status", "gproxy network firewall apply":
		return renderFirewall(w, p, data)
	case "gproxy network bbr status", "gproxy network bbr enable", "gproxy network bbr disable":
		return renderBBR(w, p, fields)
	case "gproxy network fail2ban status", "gproxy network fail2ban enable", "gproxy network fail2ban disable":
		return renderFail2ban(w, p, data)
	// The mutations below rendered as an indented JSON object until now. A
	// removal that found nothing to remove read exactly like one that worked,
	// which is the whole reason these are worth a line each.
	case "gproxy user add", "gproxy user rename", "gproxy user remove":
		return renderUser(w, p, fields)
	case "gproxy protocol add":
		return renderNodeChange(w, p, data)
	case "gproxy protocol remove":
		return renderNodeRemoval(w, p, fields)
	case "gproxy route rule add", "gproxy route rule modify", "gproxy route rule remove":
		return renderRuleResult(w, p, fields)
	case "gproxy route chain add", "gproxy route chain modify":
		return renderChainChange(w, p, data)
	case "gproxy route chain remove":
		return renderChainRemoval(w, p, fields)
	case "gproxy network firewall release":
		return renderFirewallRelease(w, p, fields)
	case "gproxy network firewall add", "gproxy network firewall remove":
		return renderPorts(w, p, fields)
	case "gproxy cert status", "gproxy cert ensure":
		return renderCertificate(w, p, data)
	case "gproxy cert port":
		return renderCaddyPort(w, p, fields)
	case "gproxy core update":
		return renderCoreUpdate(w, p, fields)
	case "gproxy update":
		return renderSelfUpdate(w, p, data)
	case "gproxy uninstall":
		return renderUninstall(w, p, fields)
	case "gproxy init":
		return renderInit(w, p, fields)
	}
	return false
}

// outcome is the word a result ends on when it says whether the target was
// actually touched. Empty when the payload carries no such field, which is
// every operation that cannot quietly do nothing.
func outcome(p palette, fields map[string]any, done, missing string) string {
	removed, present := fields["removed"].(bool)
	if !present {
		return ""
	}
	if removed {
		return p.running(done)
	}
	return p.unknown(missing)
}

// renderUser is one line in the user list's colour: the name and what
// happened to it. A user owns its exclusive nodes and its rules, so a removal
// counts what went with it, and the name is struck through as protocol
// remove strikes a membership.
func renderUser(w io.Writer, p palette, fields map[string]any) bool {
	name, ok := fields["user"].(string)
	if !ok {
		return false
	}
	user := p.user(clean(name))
	// Renaming to the same name changes nothing and says so.
	if previous, isRename := fields["previous"].(string); isRename {
		if renamed, _ := fields["renamed"].(bool); !renamed {
			fmt.Fprintln(w, user+" "+note(p, "unchanged"))
			return true
		}
		fmt.Fprintln(w, p.user(clean(previous))+" -> "+user+" "+note(p, "changed"))
		return true
	}
	if added, isAdd := fields["added"].(bool); isAdd {
		note := "added"
		if !added {
			note = "already exists"
		}
		fmt.Fprintln(w, user+" "+remark(p, note))
		return true
	}
	removed, isRemoval := fields["removed"].(bool)
	if !isRemoval {
		return false
	}
	if !removed {
		fmt.Fprintln(w, user+" "+note(p, "not found"))
		return true
	}
	with := []string{}
	for _, cascade := range []struct{ noun, key string }{{"node", "nodes_removed"}, {"rule", "rules_removed"}} {
		if count, _ := fields[cascade.key].(int); count > 0 {
			noun := cascade.noun
			if count > 1 {
				noun += "s"
			}
			with = append(with, fmt.Sprintf("%d %s", count, noun))
		}
	}
	note := "removed"
	if len(with) > 0 {
		note += "; " + strings.Join(with, ", ")
	}
	if !p.on {
		fmt.Fprintln(w, clean(name)+" ("+note+")")
		return true
	}
	fmt.Fprintln(w, struck(p, user)+" "+remark(p, note))
	return true
}

// renderNodeChange prints an installed node as the listing prints it, so the
// node just created is recognisable in `protocol list` without translation.
func renderNodeChange(w io.Writer, p palette, data any) bool {
	node, ok := data.(application.ProtocolNode)
	// An install names the user it enrolled: the answer is that user's new
	// membership as `user list` shows it, with what happened beside it.
	if ok && node.Added != "" {
		note := "added"
		if node.AlreadyMember {
			note = "already a member"
		}
		fmt.Fprintln(w, p.user(clean(node.Added))+": "+membershipLine(p, application.NodeMembership(&node))+" "+remark(p, note))
		return true
	}
	if ok {
		portWidth, securityWidth := nodeColumnWidths([]application.ProtocolNode{node})
		return writeRows(w, p, []row{nodeRow(p, node, portWidth, securityWidth)})
	}
	// The install completed but the node was not in the snapshot afterwards;
	// the handler falls back to naming what it asked for.
	fields, ok := data.(map[string]any)
	if !ok {
		return false
	}
	rows := []row{}
	for _, key := range []string{"tag", "port", "user"} {
		if value := text(fields[key]); value != "" {
			rows = append(rows, row{key, p.sys(value)})
		}
	}
	if len(rows) == 0 {
		return false
	}
	return writeRows(w, p, rows)
}

// renderNodeRemoval answers what went. Removing one member of a shared node
// leaves the node listening for everyone else, and saying "node removed" for
// both outcomes described the wrong one half the time.
func renderNodeRemoval(w io.Writer, p palette, fields map[string]any) bool {
	tag, ok := fields["tag"].(string)
	if !ok {
		return false
	}
	// A removal that took something answers the way `user list` reads: each
	// user and the membership that went, struck through, or marked
	// "(removed)" without colour.
	if gone, _ := fields["removed_memberships"].([]application.UserView); len(gone) > 0 {
		for _, view := range gone {
			for _, membership := range view.Memberships {
				line := membershipLine(p, membership)
				if view.Name != "" {
					line = p.user(clean(view.Name)) + ": " + line
				}
				if _, err := fmt.Fprintln(w, struck(p, line)); err != nil {
					return true
				}
			}
		}
		return true
	}
	// Nothing removed for a named user: the same line, not struck, with the
	// reason beside it.
	if membership, ok := fields["membership"].(application.UserMembership); ok {
		if name, _ := fields["user"].(string); name != "" {
			reason := "not a member"
			if exists, _ := fields["user_exists"].(bool); !exists {
				reason = "no such user"
			}
			fmt.Fprintln(w, p.user(clean(name))+": "+membershipLine(p, membership)+" "+note(p, reason))
			return true
		}
	}
	name, member := fields["user"].(string)
	member = member && name != ""
	state := outcome(p, fields, "removed", "not found")
	if member {
		// node_removed is present once the node itself was found, so its
		// absence is what says the tag named nothing.
		switch gone, found := fields["node_removed"].(bool); {
		case !found:
			state = p.unknown("not found")
		case gone:
			state = p.running("removed") + gap + p.hint("last member")
		default:
			state = p.unknown("kept")
		}
	}
	rows := []row{{"node", joinFields(p.label(clean(tag)), state)}}
	if member {
		rows = append(rows, row{"user", joinFields(p.count(clean(name)), outcome(p, fields, "removed", "not a member"))})
	}
	return writeRows(w, p, rows)
}

func renderChainChange(w io.Writer, p palette, data any) bool {
	chain, ok := data.(application.ChainView)
	if !ok {
		return false
	}
	fmt.Fprintln(w, chainLine(p, chain, 0))
	return true
}

func renderChainRemoval(w io.Writer, p palette, fields map[string]any) bool {
	tag, ok := fields["tag"].(string)
	if !ok {
		return false
	}
	return writeRows(w, p, []row{{"chain", joinFields(p.label(clean(tag)), outcome(p, fields, "removed", "not found"))}})
}

// renderFirewallRelease is one line: the table went, or there was none.
func renderFirewallRelease(w io.Writer, p palette, fields map[string]any) bool {
	if _, ok := fields["managed"].(bool); !ok {
		return false
	}
	fmt.Fprintln(w, "firewall  "+outcome(p, fields, "removed", "not applied"))
	return true
}

// renderPorts answers add and remove per port, the way protocol add and
// remove answer per membership: the port and "custom" with what happened
// beside it, and a removed port struck through. A port saved while the table
// is not applied says so, since nothing is open yet.
func renderPorts(w io.Writer, p palette, fields map[string]any) bool {
	changes, ok := fields["changes"].([]application.FirewallPortChange)
	if !ok {
		return false
	}
	managed, _ := fields["managed"].(bool)
	for _, change := range changes {
		entry := p.port(fmt.Sprintf("%d/%s", change.Port, clean(change.Transport))) + "  " + p.hint("custom")
		switch change.Result {
		case "removed":
			fmt.Fprintln(w, struck(p, entry))
			continue
		case "added":
			if !managed {
				fmt.Fprintln(w, entry+" "+note(p, "added; firewall not applied"))
				continue
			}
		}
		fmt.Fprintln(w, entry+" "+note(p, change.Result))
	}
	return true
}

func renderCertificate(w io.Writer, p palette, data any) bool {
	status, ok := data.(cert.Status)
	if !ok {
		fields, isMap := data.(map[string]any)
		if !isMap {
			return false
		}
		domain, hasDomain := fields["domain"].(string)
		if !hasDomain {
			return false
		}
		ready, _ := fields["ready"].(bool)
		status = cert.Status{Domain: domain, Ready: ready}
	}
	if status.Domain == "" {
		fmt.Fprintln(w, "domain: "+p.hint("none configured"))
		return true
	}
	fmt.Fprintln(w, "domain: "+renderCert(p, status))
	return true
}

// renderCaddyPort is one line: the port caddy serves its site on, and after
// a move the port it left.
func renderCaddyPort(w io.Writer, p palette, fields map[string]any) bool {
	port, ok := fields["port"].(int)
	if !ok {
		return false
	}
	line := "caddy -> " + p.port(strconv.Itoa(port))
	if previous, moved := fields["previous"].(int); moved {
		if previous == port {
			line += " " + note(p, "already there")
		} else {
			line += " " + note(p, "moved from "+strconv.Itoa(previous))
		}
	}
	fmt.Fprintln(w, line)
	return true
}

// renderCoreUpdate is one line per core the update looked at, names aligned:
// the version it moved from and to, or the version it already had.
func renderCoreUpdate(w io.Writer, p palette, fields map[string]any) bool {
	results, ok := fields["results"].([]application.CoreUpdateResult)
	if !ok {
		return false
	}
	if len(results) == 0 {
		fmt.Fprintln(w, p.hint("no installed core to update"))
		return true
	}
	width := 0
	for _, result := range results {
		width = max(width, displayWidth(clean(string(result.Component))))
	}
	for _, result := range results {
		detail := p.running(clean(result.To)) + " " + note(p, "up to date")
		if result.Updated {
			detail = p.hint(clean(result.From)) + " -> " + p.running(clean(result.To))
		}
		fmt.Fprintln(w, pad(clean(string(result.Component)), width)+"  "+detail)
	}
	return true
}

func renderSelfUpdate(w io.Writer, p palette, data any) bool {
	check, ok := data.(*update.SelfUpdateCheck)
	if !ok || check == nil {
		return false
	}
	line := "go-proxy-cli version: "
	current, latest := versionLabel(check.CurrentVersion), versionLabel(check.LatestVersion)
	switch {
	case check.Updated:
		line += p.hint(current) + " -> " + p.running(latest) + " " + note(p, "updated")
	case check.UpdateAvail:
		remarkText := "updates available"
		// --version can name an older release: moving to it is a downgrade.
		if semver.IsValid(current) && semver.Compare(latest, current) < 0 {
			remarkText = "downgrade available"
		}
		line += p.hint(current) + " -> " + p.sys(latest) + " " + note(p, remarkText)
	case !semver.IsValid(current):
		// A development build has no place in the release order to compare.
		line += p.sys(current) + " " + note(p, "development build; latest "+latest)
	default:
		line += p.running(current) + " " + note(p, "already latest version")
	}
	fmt.Fprintln(w, line)
	return true
}

// versionLabel spells a release version with its v, as the tags do; a build
// stamped without one, or a development label, is shown as it is.
func versionLabel(version string) string {
	version = clean(strings.TrimSpace(version))
	if version != "" && version[0] >= '0' && version[0] <= '9' {
		return "v" + version
	}
	return version
}

// renderUninstall lists what uninstall removes, one path per line, grouped by
// the folder it sits in with each folder in its own colour, so the units,
// the runtime and the files placed elsewhere read as separate sets. The
// firewall table and the bashrc block are not files of their own and say
// what they are beside them.
func renderUninstall(w io.Writer, p palette, fields map[string]any) bool {
	paths, ok := fields["paths"].([]string)
	if !ok {
		return false
	}
	folders := []string{}
	byFolder := map[string][]string{}
	for _, path := range paths {
		folder := filepath.Dir(path)
		if _, seen := byFolder[folder]; !seen {
			folders = append(folders, folder)
		}
		byFolder[folder] = append(byFolder[folder], path)
	}
	colours := []string{ansiBackend, ansiPort, ansiProtocol, ansiSystem, ansiOK, ansiSys}
	lines := 0
	for index, folder := range folders {
		colour := colours[index%len(colours)]
		for _, path := range byFolder[folder] {
			fmt.Fprintln(w, p.wrap(colour, clean(path)))
			lines++
		}
	}
	if block := text(fields["bashrc_block"]); block != "" {
		fmt.Fprintln(w, clean(block)+" "+note(p, "go-proxy completion block"))
		lines++
	}
	if table := text(fields["firewall_table"]); table != "" {
		fmt.Fprintln(w, clean(table)+" "+note(p, "nftables table"))
		lines++
	}
	if lines == 0 {
		fmt.Fprintln(w, p.hint("nothing to remove"))
	}
	return true
}

func renderInit(w io.Writer, p palette, fields map[string]any) bool {
	runtime, ok := fields["runtime"].(string)
	if !ok {
		return false
	}
	return writeRows(w, p, []row{{"runtime", p.sys(clean(runtime))}})
}

// renderServiceTable takes either the status map or the bare slice a service
// action returns, so starting a service reads like listing it.
func renderServiceTable(w io.Writer, p palette, data any) bool {
	states, ok := data.([]service.Status)
	if !ok {
		fields, isMap := data.(map[string]any)
		if !isMap {
			return false
		}
		if states, ok = fields["services"].([]service.Status); !ok {
			return false
		}
	}
	rows := make([]row, 0, len(states))
	for _, state := range states {
		rows = append(rows, row{serviceName(clean(string(state.Name))), serviceState(p, state)})
	}
	return writeRows(w, p, rows)
}

// bannedSample bounds the addresses the fail2ban row prints. Without it a host
// that has been scanned for a week puts several hundred on one line.
const bannedSample = 5

// gap separates fields inside one row. Wide enough to read as a column break
// without aligning to a width the data may exceed.
const gap = "    "

func renderStatus(w io.Writer, p palette, fields map[string]any) bool {
	rows := []row{}
	add := func(label, value string) {
		if value != "" {
			rows = append(rows, row{label, value})
		}
	}

	if system, ok := fields["system"].(map[string]any); ok {
		// "Debian GNU/Linux 12 (bookworm)": the codename is a note on the
		// release, drawn like every other bracketed note.
		version := clean(text(system["version"]))
		if release, codename, found := strings.Cut(version, " ("); found && strings.HasSuffix(codename, ")") {
			add("system", p.system(release)+" "+note(p, strings.TrimSuffix(codename, ")")))
		} else {
			add("system", p.system(version))
		}
	}
	if observed, ok := fields["network"].(network.Observation); ok {
		add("network", renderAddresses(p, observed))
	}
	add("protocol", renderMemberships(p, fields))
	if states, ok := fields["services"].([]service.Status); ok && len(states) > 0 {
		add("service", renderServices(p, states))
	}
	if certificate, ok := fields["cert"].(cert.Status); ok && certificate.Domain != "" {
		add("domain", renderCert(p, certificate))
	}
	for _, issue := range issues(fields) {
		add("issue", p.bad(clean(issue)))
	}
	return writeRows(w, p, rows)
}

// renderAddresses lists the addresses this host has: the best one per family
// from the interfaces, plus the public address status looked up for a NATed
// family, in that order. A probed
// address replaces a private interface address of the same family: behind NAT
// the private one is not an address anyone reaches this host on. A private
// address is still shown, marked internal, when no probe answered, because
// reporting nothing there would be worse than reporting what the host has.
//
// An address seen twice is printed once, which is the usual IPv6 case: it is
// not translated, so the interface and the probe return the same value.
func renderAddresses(p palette, observed network.Observation) string {
	best := map[string]string{}
	rank := map[string]int{"global": 3, "private": 2, "other": 1}
	for _, address := range observed.Addresses {
		if address.Scope == "loopback" || address.Scope == "link_local" {
			continue
		}
		if best[address.Family] == "" || rank[address.Scope] > rank[scopeOf(observed, best[address.Family])] {
			best[address.Family] = address.Address
		}
	}
	probes := map[string]network.Probe{"ipv4": observed.IPv4, "ipv6": observed.IPv6}
	parts := []string{}
	seen := map[string]bool{}
	for _, family := range []string{"ipv4", "ipv6"} {
		local := trimPrefixLen(best[family])
		probed := clean(probes[family].Address)
		internal := local != "" && scopeOf(observed, best[family]) != "global"
		if internal && probed != "" {
			local = ""
		}
		for _, value := range []string{clean(local), probed} {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			shown := p.network(value)
			if value == clean(local) && internal {
				shown += " " + note(p, "internal")
			}
			parts = append(parts, shown)
		}
	}
	if len(parts) == 0 {
		return p.unknown("none")
	}
	return strings.Join(parts, p.hint(" / "))
}

func scopeOf(observed network.Observation, address string) string {
	for _, candidate := range observed.Addresses {
		if candidate.Address == address {
			return candidate.Scope
		}
	}
	return ""
}

// trimPrefixLen drops the /NN suffix: the dashboard reports which address the
// host has, not how its subnet is sized.
func trimPrefixLen(address string) string {
	if index := strings.LastIndex(address, "/"); index > 0 {
		return address[:index]
	}
	return address
}

// renderMemberships answers "who has what", which a node count never did.
func renderMemberships(p palette, fields map[string]any) string {
	byUser, ok := fields["memberships"].(map[string][]string)
	if !ok || len(byUser) == 0 {
		return ""
	}
	names := make([]string, 0, len(byUser))
	for name := range byUser {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		protocols := byUser[name]
		if len(protocols) == 0 {
			parts = append(parts, p.protocol(clean(name))+p.hint(":")+p.unknown("none"))
			continue
		}
		cleaned := make([]string, 0, len(protocols))
		for _, protocol := range protocols {
			cleaned = append(cleaned, p.protocol(clean(protocol)))
		}
		parts = append(parts, p.protocol(clean(name))+p.hint(":")+strings.Join(cleaned, p.hint("/")))
	}
	return strings.Join(parts, gap)
}

// renderServices names each managed service and its state. Colour carries the
// state where there is colour; where there is not -- a pipe, a file, NO_COLOR,
// --no-color -- the state is a word, because five names with nothing beside
// them answered none of what the row exists to answer.
func renderServices(p palette, states []service.Status) string {
	parts := make([]string, 0, len(states))
	for _, state := range states {
		name := serviceName(clean(string(state.Name)))
		switch {
		case !state.Installed:
			parts = append(parts, p.unknown(name+stateWord(p, "absent")))
		case state.Running:
			parts = append(parts, p.running(name+stateWord(p, "running")))
		default:
			parts = append(parts, p.stopped(name+stateWord(p, "stopped")))
		}
	}
	return strings.Join(parts, p.hint("/"))
}

// note is a value's bracketed remark, in lowercase: every parenthesised note
// in human output goes through it, written one space after its value.
func note(p palette, text string) string {
	return p.wrap(ansiNote, "("+strings.ToLower(clean(text))+")")
}

// remark is note for a text that already carries its brackets, as the user
// results build them.
func remark(p palette, text string) string {
	return note(p, strings.TrimSuffix(strings.TrimPrefix(text, "("), ")"))
}

func stateWord(p palette, state string) string {
	if p.on {
		return ""
	}
	return " (" + state + ")"
}

func renderCert(p palette, certificate cert.Status) string {
	domain := p.sys(clean(certificate.Domain))
	// A healthy certificate's note is an ordinary note. A warning keeps its
	// red inside the note's brackets: a certificate about to lapse must not
	// read like one that is fine.
	healthy := func(_ func(string) string, text string) string { return domain + " " + note(p, text) }
	warning := func(colour func(string) string, text string) string {
		return domain + " " + p.wrap(ansiNote, "(") + colour(text) + p.wrap(ansiNote, ")")
	}
	detail := warning
	if !certificate.Ready {
		return detail(p.stopped, "not issued")
	}
	if certificate.ExpiresAt == nil {
		return healthy(p.running, "ready")
	}
	left := time.Until(*certificate.ExpiresAt)
	if left < 0 {
		return detail(p.stopped, "expired")
	}
	if left < 24*time.Hour {
		return detail(p.stopped, "expires in under a day")
	}
	// Rounded, not truncated: a certificate 89 days out reported as 88 reads as
	// an error in the display rather than as the deliberate margin it is not.
	days := int(math.Round(left.Hours() / 24))
	remaining := fmt.Sprintf("expires in %d days", days)
	if days == 1 {
		remaining = "expires in 1 day"
	}
	if days <= 14 {
		return detail(p.stopped, remaining)
	}
	return healthy(p.running, remaining)
}

func issues(fields map[string]any) []string {
	found, ok := fields["issues"].([]string)
	if !ok {
		return nil
	}
	return found
}

func text(value any) string {
	if value == nil {
		return ""
	}
	return clean(fmt.Sprint(value))
}

// clean removes escape sequences and C0 control characters from values that
// originate in systemd output, configuration, a log or an error message.
// Without it a crafted unit name or issue text could inject an escape sequence
// into a rendering that reports itself as colour-free, which is exactly the
// guarantee the --no-color and non-terminal paths make.
//
// Whole sequences go, not just the escape byte: sing-box and caddy colour their
// own logs, and dropping only the ESC would leave "[33m" in front of every line
// it wrote. The same stripping runs where a log is read, so `--json` carries no
// escapes either; this is the display half of one guarantee.
func clean(value string) string { return textutil.Clean(value) }

// pad counts runes, not bytes. A firewall source such as "shadow-tls→snell"
// is two bytes wider than it looks, and padding it by length pulled the column
// after it two places left.
func pad(label string, width int) string {
	size := displayWidth(label)
	if size >= width {
		return label
	}
	return label + strings.Repeat(" ", width-size)
}

// displayWidth is what pad measures, so a renderer sizing its own column
// measures the same thing.
//
// Escape sequences take no columns, so a label coloured by its caller is
// measured by what it shows.
func displayWidth(value string) int {
	return utf8.RuneCountInString(escapeSequence.ReplaceAllString(value, ""))
}

var escapeSequence = regexp.MustCompile("\x1b\\[[0-9;]*m")

// renderUsers groups each user's protocols under the name, one bullet each,
// as the v0.1.59 overview did. It is the one listing without row numbers: no
// command takes a membership by position, and a user is selected by name.
func renderUsers(w io.Writer, p palette, fields map[string]any) bool {
	users, ok := fields["users"].([]application.UserView)
	if !ok {
		return false
	}
	if len(users) == 0 {
		fmt.Fprintln(w, p.hint("no users"))
		return true
	}
	for _, user := range users {
		line := p.user(clean(user.Name))
		if len(user.Memberships) == 0 {
			line += "  " + p.unknown("no protocols")
		}
		if user.Routes > 0 {
			line += "  " + p.count(fmt.Sprintf("%d", user.Routes)) + p.hint(" routes")
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return true
		}
		for _, m := range user.Memberships {
			if _, err := fmt.Fprintln(w, p.ok("●")+" "+membershipLine(p, m)); err != nil {
				return true
			}
		}
	}
	return true
}

// struck draws a whole line struck through, colours kept. Every coloured piece
// ends in a reset that would also end the strike, so the strike is taken up
// again after each one. Without colour the line is marked "(removed)".
func struck(p palette, line string) string {
	if !p.on {
		return line + " (removed)"
	}
	const strike = "\x1b[9m"
	return strike + strings.ReplaceAll(line, ansiReset, ansiReset+strike) + ansiReset
}

// membershipLine names a membership and the port a client dials. A node behind
// ShadowTLS is dialled at the wrapper, so both ports are shown along the path
// the traffic takes.
func membershipLine(p palette, m application.UserMembership) string {
	name := clean(m.Protocol)
	if m.ShadowTLS == nil {
		return p.ok(name) + " - " + p.port(fmt.Sprintf("%d", m.Port))
	}
	wrapper := "shadow-tls"
	if m.ShadowTLS.Version > 0 {
		wrapper += fmt.Sprintf("-v%d", m.ShadowTLS.Version)
	}
	backend := name
	if m.Protocol == store.SnellTag {
		backend = "snell"
	}
	return p.ok(name+"+"+wrapper) + " - shadow-tls:" + p.port(fmt.Sprintf("%d", m.ShadowTLS.Port)) +
		" -> " + backend + ":" + p.backend(fmt.Sprintf("%d", m.Port))
}

// nodeSecurity names the layer that encrypts a node. A wrapped node is
// encrypted by its wrapper, named without its port: the listing answers what a
// node is, and the one port worth naming is the node's own.
func nodeSecurity(node application.ProtocolNode) string {
	if node.ShadowTLS != nil {
		return "shadow-tls"
	}
	if node.Security == "none" {
		return ""
	}
	return clean(node.Security)
}

func nodeOwners(node application.ProtocolNode) []string {
	owners := make([]string, 0, len(node.Users))
	for _, name := range node.Users {
		owners = append(owners, clean(name))
	}
	return owners
}

// nodeColumnWidths sizes the two middle columns across a whole listing, so the
// listing and the removal guidance lay a node out the same way.
func nodeColumnWidths(nodes []application.ProtocolNode) (port, security int) {
	for _, node := range nodes {
		if size := displayWidth(fmt.Sprintf("%d", node.Port)); size > port {
			port = size
		}
		if size := displayWidth(nodeSecurity(node)); size > security {
			security = size
		}
	}
	return port, security
}

// renderCatalogue lists what can be installed: the name and the forms it comes
// in, one per row. The prose belongs to the guidance a reader sees when they do
// not yet know what to pick.
func renderCatalogue(w io.Writer, p palette, fields map[string]any) bool {
	entries, ok := fields["protocols"].([]map[string]string)
	if !ok {
		return false
	}
	rows := make([]row, 0, len(entries))
	for _, entry := range entries {
		display := entry["display"]
		if display == "" {
			display = entry["name"]
		}
		// "vless (tls/reality)": the forms it comes in are a note on the name.
		label := clean(display)
		if name, variants, found := strings.Cut(display, " ("); found {
			label = clean(name) + " " + note(p, strings.TrimSuffix(variants, ")"))
		}
		rows = append(rows, row{label, ""})
	}
	return writeRows(w, p, rows)
}

// nodeRow is one installed-node line: the protocol, the port it listens on, the
// layer that encrypts it, and who it belongs to. Shared by the rendering of a
// node just installed and by the listing `protocol remove` prints when it asks
// which node to remove, so the two cannot describe one node differently.
func nodeRow(p palette, node application.ProtocolNode, portWidth, securityWidth int) row {
	owners := nodeOwners(node)
	security := nodeSecurity(node)
	// Padded only while something follows, so no line ends in whitespace.
	detail := p.count(pad(fmt.Sprintf("%d", node.Port), portWidth))
	if security != "" || len(owners) > 0 {
		detail += gap + p.hint(pad(security, securityWidth))
	}
	if len(owners) > 0 {
		detail += gap + strings.Join(owners, p.hint("/"))
	}
	return row{clean(node.Type), detail}
}

func renderCores(w io.Writer, p palette, data any) bool {
	switch items := data.(type) {
	case []core.VersionInfo:
		rows := make([]row, 0, len(items))
		for _, item := range items {
			detail := p.unknown("not installed")
			if item.Installed {
				detail = p.running(clean(item.Version))
			}
			rows = append(rows, row{clean(string(item.Component)), detail})
		}
		return writeAligned(w, p, rows)
	case []*core.UpdateCheck:
		rows := make([]row, 0, len(items))
		for _, item := range items {
			// The version first, then what it means in a note:
			// "1.14.1 -> v1.14.2 (update available)", "0.2.25 (up to date)".
			var detail string
			switch {
			case !item.Installed:
				// Nothing installed has nothing to update; reporting an update
				// here would send the reader to `core update` for a first install.
				detail = p.unknown("not installed") + " " + note(p, "latest "+item.LatestVersion)
			case item.CurrentVersion == "":
				// Installed but unreadable: the binary is there and did not
				// answer --version, which is not the same as absent.
				detail = p.stopped("version unknown") + " " + note(p, "latest "+item.LatestVersion)
			case item.UpdateAvail:
				detail = p.hint(clean(item.CurrentVersion)) + " -> " + p.sys(clean(item.LatestVersion)) + " " + note(p, "update available")
			default:
				detail = p.running(clean(item.CurrentVersion)) + " " + note(p, "up to date")
			}
			rows = append(rows, row{clean(string(item.Component)), detail})
		}
		return writeAligned(w, p, rows)
	}
	return false
}

// renderRules groups by user because a rule belongs to one, and because the
// index beside it is only meaningful within that user.
func renderRules(w io.Writer, p palette, fields map[string]any) bool {
	entries, ok := fields["rules"].([]application.RouteEntry)
	if !ok {
		return false
	}
	return writeRuleList(w, p, entries, nil, nil)
}

// renderRuleResult answers add, modify and remove with the user's rules as
// they now stand, each pointing at where its traffic goes, so the effect of a
// change is read in place. Rules a remove took out are listed too, struck
// through; without colour they are marked "(removed)".
func renderRuleResult(w io.Writer, p palette, fields map[string]any) bool {
	entries, ok := fields["rules"].([]application.RouteEntry)
	if !ok {
		return false
	}
	removed, _ := fields["removed"].([]application.RouteEntry)
	// Presets an add found already there are shown where they stand, struck
	// through: nothing about them changed, and the note says why.
	already := map[string]bool{}
	if names, ok := fields["already_added"].([]string); ok {
		for _, name := range names {
			already[name] = true
		}
	}
	return writeRuleList(w, p, entries, removed, already)
}

// writeRuleList groups rules under their user, numbered with the preset menu
// index --rules takes and in menu order; removed rules take their old place.
func writeRuleList(w io.Writer, p palette, entries, removed []application.RouteEntry, already map[string]bool) bool {
	if len(entries) == 0 && len(removed) == 0 {
		fmt.Fprintln(w, p.hint("no rules"))
		return true
	}
	type line struct {
		entry   application.RouteEntry
		removed bool
	}
	byUser := map[string][]line{}
	names := []string{}
	add := func(entry application.RouteEntry, gone bool) {
		if _, seen := byUser[entry.User]; !seen {
			names = append(names, entry.User)
		}
		byUser[entry.User] = append(byUser[entry.User], line{entry, gone})
	}
	for _, entry := range entries {
		add(entry, false)
	}
	for _, entry := range removed {
		add(entry, true)
	}
	sort.Strings(names)
	sections := make([]section, 0, len(names))
	for _, name := range names {
		lines := byUser[name]
		sort.SliceStable(lines, func(i, j int) bool {
			return routing.PresetRank(lines[i].entry.Preset) < routing.PresetRank(lines[j].entry.Preset)
		})
		current := section{title: p.user(clean(name)) + p.hint(":")}
		for _, item := range lines {
			entry := item.entry
			mark := entry.Selector
			if mark == "" {
				mark = strconv.Itoa(entry.Index)
			}
			if item.removed || already[entry.Preset] && entry.Preset != "" {
				label, target := clean(entry.Label), ruleTarget(palette{}, entry.Outbound, entry.Address)
				note := "(removed)"
				if !item.removed {
					note = "(already added)"
				}
				switch {
				case !p.on:
					current.rows = append(current.rows, row{label, target + " " + note})
					current.marks = append(current.marks, mark)
				case item.removed:
					current.rows = append(current.rows, row{p.wrap(ansiRemoved, label), p.wrap(ansiRemoved, target)})
					current.marks = append(current.marks, p.wrap(ansiRemoved, mark))
				default:
					// Struck, and the reason beside it: a strike alone would
					// read as a removal.
					current.rows = append(current.rows, row{p.wrap(ansiRemoved, label), p.wrap(ansiRemoved, target) + " " + remark(p, note)})
					current.marks = append(current.marks, p.wrap(ansiRemoved, mark))
				}
				continue
			}
			current.rows = append(current.rows, row{p.wrap(presetColour(entry.Preset), clean(entry.Label)), ruleTarget(p, entry.Outbound, entry.Address)})
			current.marks = append(current.marks, mark)
		}
		sections = append(sections, current)
	}
	return writeSections(w, p, sections)
}

// ruleTarget is where a rule sends its traffic, as shell-proxy printed it:
// direct on its own, or a chain as "tag: host:port".
func ruleTarget(p palette, outbound, address string) string {
	outbound = clean(outbound)
	if outbound == store.DirectTag {
		return "-> " + p.wrap(ansiDirect, outbound)
	}
	line := "-> " + p.wrap(ansiChainTag, outbound)
	if address != "" {
		line += ": " + p.wrap(ansiChainAddress, clean(address))
	}
	return line
}

// target is the "-> where it goes" half of a line, shared by the rule and chain
// listings so one reads like the other. A chain names only its address; a rule
// names the outbound it selected, and the address behind it when that outbound
// is a chain rather than direct egress.
func target(p palette, outbound, address string) string {
	parts := []string{}
	if outbound != "" {
		parts = append(parts, p.label(clean(outbound)))
	}
	if address != "" {
		parts = append(parts, p.sys(clean(address)))
	}
	return p.hint("-> ") + strings.Join(parts, gap)
}

func renderChains(w io.Writer, p palette, fields map[string]any) bool {
	chains, ok := fields["chains"].([]application.ChainView)
	if !ok {
		return false
	}
	if len(chains) == 0 {
		fmt.Fprintln(w, p.hint("no chain outbounds"))
		return true
	}
	width := 0
	for _, chain := range chains {
		width = max(width, displayWidth(clean(chain.Tag)))
	}
	for _, chain := range chains {
		fmt.Fprintln(w, chainLine(p, chain, width))
	}
	return true
}

// chainLine is one chain as a rule names it, "res1 -> host:port", with how
// its lookups are made in brackets: the address family and the resolver
// reached through it. Who selects the chain is route rule list's to say. A
// chain that is the route final says so, since it then carries everything.
func chainLine(p palette, chain application.ChainView, width int) string {
	line := pad(p.wrap(ansiChainTag, clean(chain.Tag)), width) + " -> " + p.wrap(ansiChainAddress, clean(chain.Address))
	lookups := []string{}
	for _, part := range []string{chain.DomainStrategy, chain.Resolver} {
		if part != "" {
			lookups = append(lookups, clean(part))
		}
	}
	if len(lookups) > 0 {
		line += " " + note(p, strings.Join(lookups, "/"))
	}
	if chain.Final {
		line += " " + p.sys("final")
	}
	return line
}

// renderFinal is one line: where unmatched traffic leaves, as a rule line
// names its target, then the DNS server its names are resolved by.
func renderFinal(w io.Writer, p palette, fields map[string]any) bool {
	final, ok := fields["final"].(string)
	if !ok {
		return false
	}
	address, _ := fields["address"].(string)
	line := "final " + ruleTarget(p, final, address)
	if dns, _ := fields["dns_final"].(string); dns != "" {
		line += " " + note(p, "dns "+dns)
	}
	fmt.Fprintln(w, line)
	return true
}

func renderDirect(w io.Writer, p palette, fields map[string]any) bool {
	strategy, ok := fields["strategy"].(string)
	if !ok {
		return false
	}
	fmt.Fprintln(w, "direct -> "+p.sys(clean(strategy)))
	return true
}

// renderBBR is one line: whether TCP uses BBR, and in the note what keeps
// it at boot -- gproxy's own file, other sysctl files, or nothing, in which
// case a reboot turns it off. A disable that another file will undo at boot
// says so, since the running kernel alone does not tell the whole story.
func renderBBR(w io.Writer, p palette, fields map[string]any) bool {
	current, ok := fields["current"].(string)
	if !ok {
		return false
	}
	enabled, _ := fields["enabled"].(bool)
	managed, _ := fields["managed"].(bool)
	sources, _ := fields["boot_sources"].([]string)
	change, _ := fields["change"].(string)
	holders := append([]string{}, sources...)
	if managed {
		holders = append([]string{"gproxy"}, holders...)
	}
	keptBy := strings.Join(holders, ", ")
	if keptBy == "" {
		keptBy = "not kept at boot"
	}
	using := "using " + current
	if len(sources) > 0 {
		using += "; " + strings.Join(sources, ", ") + " enables it at boot"
	}
	var line, remarkText string
	switch {
	case change == "disabled":
		line, remarkText = "bbr "+p.stopped("disabled"), using
	case change == "already disabled":
		line, remarkText = "bbr "+p.hint("already disabled"), using
	case change == "already enabled":
		line, remarkText = "bbr "+p.running("already enabled"), keptBy
	case enabled:
		line, remarkText = "bbr "+p.running("enabled"), keptBy
	default:
		line, remarkText = "bbr "+p.stopped("not enabled"), using
	}
	if current == "" && !enabled {
		remarkText = ""
	}
	if remarkText != "" {
		line += " " + note(p, remarkText)
	}
	fmt.Fprintln(w, line)
	return true
}

// renderFail2ban reports the same fields the v0.1.59 table did: the service,
// the jail, the thresholds that decide a ban, the counts and who is banned
// now. The threshold rows are skipped when fail2ban did not report them, which
// is what happens when it is not installed.
// jailOwners names what switches the sshd jail on: gproxy's own jail file,
// then any other configuration file that does.
func jailOwners(info network.Fail2BanInfo) []string {
	owners := []string{}
	if info.Managed {
		owners = append(owners, "gproxy")
	}
	return append(owners, info.JailSources...)
}

// fail2banChange is enable's and disable's one line. disable stops fail2ban
// itself, so it says how many bans that lifted; enable says whether it started
// the service or only put gproxy's jail in place.
func fail2banChange(p palette, info network.Fail2BanInfo) string {
	line := "fail2ban  "
	switch info.Change {
	case "stopped":
		detail := "service disabled"
		if info.BansLifted > 0 {
			detail += fmt.Sprintf("; %d bans lifted", info.BansLifted)
		}
		return line + p.stopped("stopped") + " " + note(p, detail)
	case "already stopped":
		return line + p.hint("already stopped")
	case "started":
		return line + p.running("running") + " " + note(p, "started with gproxy's ssh jail")
	case "added":
		return line + p.running("running") + " " + note(p, "gproxy ssh jail added")
	case "already managed":
		return line + p.running("running") + " " + note(p, "gproxy ssh jail already in place")
	}
	return line + clean(info.Change)
}

func renderFail2ban(w io.Writer, p palette, data any) bool {
	info, ok := data.(network.Fail2BanInfo)
	if !ok {
		return false
	}
	state := p.unknown("not installed")
	switch {
	case info.Installed && info.Running:
		state = p.running("running")
	case info.Installed:
		state = p.stopped("stopped")
	}
	if info.Change != "" {
		fmt.Fprintln(w, fail2banChange(p, info))
		return true
	}
	jail := p.stopped("off")
	if info.SSHJailEnabled {
		jail = p.running("on")
		// Who keeps it on, since disable removes only gproxy's own jail.
		if owners := jailOwners(info); len(owners) > 0 {
			jail += " " + note(p, strings.Join(owners, ", "))
		}
	}
	rows := []row{{"fail2ban", state}, {"ssh jail", jail}}
	for _, setting := range []struct{ label, value, unit string }{
		{"max retry", info.MaxRetry, ""},
		{"ban time", info.BanTime, "s"},
		{"find time", info.FindTime, "s"},
	} {
		if setting.value != "" {
			rows = append(rows, row{setting.label, p.sys(clean(setting.value) + setting.unit)})
		}
	}
	// A stopped fail2ban holds no bans and answers no counts: the two rows
	// above say everything.
	if info.Running {
		rows = append(rows,
			row{"banned now", p.count(fmt.Sprintf("%d", info.CurrentlyBanned))},
			row{"banned total", p.count(fmt.Sprintf("%d", info.TotalBanned))})
	}
	switch {
	case info.BannedFile != "" && len(info.BannedIPs) > 0:
		// A busy host bans hundreds, which no terminal line holds. The count
		// is above; the addresses are in the file, one per line.
		rows = append(rows, row{"banned ip", p.sys(clean(info.BannedFile))})
	case len(info.BannedIPs) > 0:
		// The file could not be written: a sample, and --json has the rest.
		shown := info.BannedIPs
		if len(shown) > bannedSample {
			shown = shown[:bannedSample]
		}
		addresses := make([]string, 0, len(shown))
		for _, address := range shown {
			addresses = append(addresses, p.stopped(clean(address)))
		}
		detail := strings.Join(addresses, p.hint("/"))
		if rest := len(info.BannedIPs) - len(shown); rest > 0 {
			detail += gap + p.hint(fmt.Sprintf("+%d more", rest))
		}
		rows = append(rows, row{"banned ip", detail})
	}
	return writeRows(w, p, rows)
}

// renderFirewall answers what `apply` would do. Each port is one row: the port
// and transport, what asked for it, and the change pending on it. A port with
// nothing pending says nothing, so the rows that need attention are the only
// ones carrying a word in the last column.
//
// The nftables ruleset itself stays in the JSON. It is the tool's own syntax,
// and reprinting it here would be quoting nft at the reader rather than
// answering the question.
// renderFirewall is a headline and the ports under it. The headline says
// whether gproxy's table is in place and, when it is, whether the kernel
// matches what gproxy wants. Unapplied or in sync, the ports listed are the
// ones gproxy keeps open; with drift, only the changes apply would make.
// The last line is the command that closes the gap, when there is one, with
// what applying does: everything else inbound is dropped.
func renderFirewall(w io.Writer, p palette, data any) bool {
	info, ok := data.(network.FirewallInfo)
	if !ok {
		return false
	}
	pending := len(info.Add) + len(info.Remove)
	state, detail := p.stopped("not applied"), p.hint("nftables available")
	switch {
	case !info.Managed && !info.Available:
		detail = p.hint("nftables not installed; apply installs it")
	case info.Managed && pending == 0:
		state, detail = p.running("applied"), p.hint("in sync")
	case info.Managed:
		noun := "changes"
		if pending == 1 {
			noun = "change"
		}
		state, detail = p.running("applied"), p.sys(fmt.Sprintf("%d %s pending", pending, noun))
	}
	fmt.Fprintln(w, "firewall  "+state+"  "+detail)

	type line struct{ mark, port, source string }
	lines := []line{}
	sources := map[string]string{}
	for _, spec := range info.Desired {
		key := fmt.Sprintf("%d/%s", spec.Port, clean(spec.Proto))
		sources[key] = clean(strings.Join(spec.Sources, ", "))
		if !info.Managed || pending == 0 {
			lines = append(lines, line{"", key, sources[key]})
		}
	}
	if info.Managed && pending > 0 {
		for _, change := range info.Add {
			key := fmt.Sprintf("%d/%s", change.Port, clean(change.Proto))
			lines = append(lines, line{p.running("+") + " ", key, sources[key]})
		}
		for _, change := range info.Remove {
			key := fmt.Sprintf("%d/%s", change.Port, clean(change.Proto))
			lines = append(lines, line{p.stopped("-") + " ", key, "no longer used"})
		}
	}
	width := 0
	for _, entry := range lines {
		width = max(width, displayWidth(entry.mark+entry.port))
	}
	for _, entry := range lines {
		fmt.Fprintln(w, pad(entry.mark+p.port(entry.port), width)+"  "+p.hint(entry.source))
	}
	switch {
	case !info.Managed && len(info.Desired) > 0:
		fmt.Fprintln(w, p.command("gproxy network firewall apply")+" "+
			note(p, fmt.Sprintf("opens these %d ports, drops other inbound", len(info.Desired))))
	case info.Managed && pending > 0:
		fmt.Fprintln(w, p.command("gproxy network firewall apply"))
	}
	return true
}

// joinFields puts the column gap between two fields, skipping it when either is
// empty: a missing first field must not indent the second, and a missing second
// must not leave the line ending in whitespace.
func joinFields(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + gap + second
}

// renderLog prints the log as a log: the header identifies what was read, then
// the lines themselves, verbatim and unnumbered. Numbering them would add this
// CLI's chrome to another program's output and break a copied or grepped line,
// and no command takes a log line number the way --rules takes a rule index.
func renderLog(w io.Writer, p palette, fields map[string]any) bool {
	content, ok := fields["content"].(string)
	if !ok {
		return false
	}
	// The selector, not the shortened display name: it sits beside the source
	// path, which carries the same unit name, and it is what --follow retakes.
	header := p.label(clean(text(fields["service"])))
	if source := clean(text(fields["source"])); source != "" {
		header += " " + p.hint(source)
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return true
	}
	body := strings.TrimRight(content, "\n")
	if body == "" {
		fmt.Fprintln(w, p.hint("no entries"))
		return true
	}
	for _, line := range strings.Split(body, "\n") {
		if _, err := fmt.Fprintln(w, logLine(p, line)); err != nil {
			return true
		}
	}
	return true
}

// logLine colours a whole line by the level it names, the way the v0.1.59
// viewer did. The level is the only part of a foreign log line this CLI can
// recognise, and a failure has to be findable by eye in fifty lines of routine
// output. Unlike that viewer, a plain informational line is left uncoloured:
// colouring every line is what stops any of them standing out.
//
// The line is stripped first, so a log that colours itself -- sing-box and
// caddy both do -- cannot paint past --no-color or into a captured file.
func logLine(p palette, line string) string {
	line = collapseSpaces(clean(line))
	upper := strings.ToUpper(line)
	for _, level := range []string{"FATAL", "PANIC", "ERROR", "FAILED"} {
		if strings.Contains(upper, level) {
			return p.bad(line)
		}
	}
	if strings.Contains(upper, "WARN") {
		return p.sys(line)
	}
	return line
}

// collapseSpaces reduces each run of spaces between words to one, which is
// only display: loggers pad a level to a fixed width ("Z  INFO"), and the
// padding reads as a gap. Leading indentation, as in a stack trace, is kept,
// and the log file and --json content are untouched.
func collapseSpaces(line string) string {
	body := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(body)]
	return indent + repeatedSpaces.ReplaceAllString(body, " ")
}

var repeatedSpaces = regexp.MustCompile(` {2,}`)

// logWriter applies renderLog's per-line treatment to a followed stream. It
// buffers only the partial line at the end of a read; a line longer than the
// cap is flushed uncoloured rather than grown without limit, because a log with
// no newline in it must not become a memory leak.
type logWriter struct {
	out     io.Writer
	p       palette
	partial []byte
}

const logLineCap = 64 << 10

func (l *logWriter) Write(data []byte) (int, error) {
	consumed := len(data)
	for {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			break
		}
		line := append(l.partial, data[:index]...)
		l.partial = nil
		data = data[index+1:]
		if _, err := fmt.Fprintln(l.out, logLine(l.p, string(line))); err != nil {
			return consumed, err
		}
	}
	l.partial = append(l.partial, data...)
	if len(l.partial) > logLineCap {
		if _, err := l.out.Write(l.partial); err != nil {
			return consumed, err
		}
		l.partial = nil
	}
	return consumed, nil
}

// Close writes whatever the stream ended on. A followed log is cancelled rather
// than finished, so the last read usually stops mid-line.
func (l *logWriter) Close() error {
	if len(l.partial) == 0 {
		return nil
	}
	_, err := fmt.Fprintln(l.out, logLine(l.p, string(l.partial)))
	l.partial = nil
	return err
}

// renderRouteTest is one line: the name, the rule that takes it -- numbered as
// route rule list numbers it, or the route final -- where the traffic goes, and
// whether the lookup leaves the same way. What matched is shown beside the rule
// in grey, since a preset's rule set is what decided, not its label.
func renderRouteTest(w io.Writer, p palette, fields map[string]any) bool {
	result, ok := fields["evaluation"].(routing.Evaluation)
	if !ok {
		return false
	}
	decision := result.Decision
	address, _ := fields["address"].(string)
	var rule string
	switch entry, matched := fields["rule"].(application.RouteEntry); {
	case matched:
		rule = entry.Selector + "." + p.wrap(presetColour(entry.Preset), clean(entry.Label))
	case decision.MatchBy == "final":
		rule = p.hint("final")
	case decision.MatchBy == "ip_is_private":
		rule = p.hint("private address")
	default:
		rule = p.hint("rule " + strconv.Itoa(decision.Rule+1))
	}
	if decision.Value != "" {
		rule += " " + note(p, decision.Value)
	}
	fmt.Fprintln(w, p.label(clean(result.Target))+"  "+rule+"  "+ruleTarget(p, decision.Outbound, address)+"  "+routeDNS(p, decision))
	if len(result.Unchecked) > 0 {
		fmt.Fprintln(w, p.hint("unless an earlier rule set matches; not in sing-box's cache yet: "+clean(strings.Join(result.Unchecked, ", "))))
	}
	return true
}

// routeDNS says whether the resolver leaves by the same outbound as the
// traffic. It is stated either way rather than only when they differ: the point
// of asking is to confirm the pair, and silence would not distinguish "checked
// and matching" from "not checked".
func routeDNS(p palette, decision routing.Decision) string {
	if decision.DNSVia == decision.Outbound || decision.DNSVia == "" && decision.Outbound == store.DirectTag {
		return p.running("dns ok")
	}
	via := decision.DNSVia
	if via == "" {
		via = store.DirectTag
	}
	return p.bad("dns via " + clean(via))
}

// renderSubscription groups the default export the way it is read: a format at
// a time, a user at a time, and the links under them. The old shape put a
// "user / tag (format)" header above every single link, which was more header
// than payload and said the same three things over and over.
//
// The mihomo block carries its own key and sequence indent, and names its users
// in YAML comments, so the section under [mihomo] loads as written. Listing the
// mappings bare read well and could not be used: mihomo needs the proxies key,
// and a reader had to re-export with --mihomo to get one.
//
// The links themselves are left uncoloured. They are what gets copied, and the
// headings are enough to find a block by eye.
func renderSubscription(w io.Writer, p palette, fields map[string]any) bool {
	links, ok := fields["links"].([]application.SubscriptionLink)
	if !ok {
		return false
	}
	if len(links) == 0 {
		fmt.Fprintln(w, p.hint("no links"))
		return true
	}
	format, user := "", ""
	for index, link := range links {
		document := link.Format == string(subscription.FormatMihomo)
		// One blank line between sections and between the entries inside
		// one, so each configuration reads as its own block. A blank line
		// inside the mihomo sequence is still valid YAML.
		if index > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return true
			}
		}
		if link.Format != format {
			format, user = link.Format, ""
			if _, err := fmt.Fprintln(w, p.sys("["+clean(format)+"]")); err != nil {
				return true
			}
			if document {
				if _, err := fmt.Fprintln(w, "proxies:"); err != nil {
					return true
				}
			}
		}
		if link.User != user {
			user = link.User
			heading := p.count(clean(user))
			if document {
				heading = p.hint("# " + clean(user))
			}
			if _, err := fmt.Fprintln(w, heading); err != nil {
				return true
			}
		}
		content := clean(link.Content)
		if document {
			content = "  - " + content
		}
		if _, err := fmt.Fprintln(w, content); err != nil {
			return true
		}
	}
	return true
}

// renderValidation is one line per checked component, each name in its own
// colour: the state, and in the note what was checked. A check that fails
// stops the command with an error, so the lines here are the checks passed.
func renderValidation(w io.Writer, p palette, fields map[string]any) bool {
	checks, ok := fields["checks"].([]map[string]string)
	if !ok {
		return false
	}
	colours := map[string]string{"sing-box": ansiBackend, "snell": ansiPort, "shadow-tls": ansiProtocol}
	checked := map[string]string{
		"core":                 "full configuration checked by sing-box itself",
		"configuration schema": "listener port and psk length",
		"binding schema":       "listener ports, sni, version and snell backend",
	}
	width := 0
	for _, check := range checks {
		width = max(width, displayWidth(clean(check["component"])))
	}
	for _, check := range checks {
		component := clean(check["component"])
		colour, known := colours[component]
		if !known {
			colour = ansiLabel
		}
		state := p.running(clean(check["state"]))
		if check["state"] != "passed" {
			state = p.stopped(clean(check["state"]))
		}
		what := checked[check["validation"]]
		if what == "" {
			what = check["validation"]
		}
		fmt.Fprintln(w, p.wrap(colour, pad(component, width))+"  "+state+" "+note(p, what))
	}
	return true
}
