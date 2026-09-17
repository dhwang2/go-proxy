package cli

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"go-proxy/internal/cert"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
)

// Truecolor SGR codes matching the v0.1.59 interface palette, so the CLI reads
// the way the terminal interface did. Colour is opt-out rather than negotiated:
// a terminal that cannot render it is excluded by the device check below, not
// by degrading to a smaller palette.
const (
	ansiReset = "\x1b[0m"
	ansiOK    = "\x1b[38;2;152;195;121m" // running, ready
	ansiBad   = "\x1b[38;2;224;108;117m" // stopped, failing
	ansiSys   = "\x1b[38;2;245;158;11m"  // system values
	ansiCount = "\x1b[38;2;97;175;239m"  // counts
	ansiLabel = "\x1b[38;2;229;231;235m" // row labels
	ansiHint  = "\x1b[38;2;107;114;128m" // secondary detail
)

type palette struct{ on bool }

func (p palette) wrap(code, text string) string {
	if !p.on || text == "" {
		return text
	}
	return code + text + ansiReset
}

func (p palette) ok(text string) string    { return p.wrap(ansiOK, text) }
func (p palette) bad(text string) string   { return p.wrap(ansiBad, text) }
func (p palette) sys(text string) string   { return p.wrap(ansiSys, text) }
func (p palette) count(text string) string { return p.wrap(ansiCount, text) }
func (p palette) label(text string) string { return p.wrap(ansiLabel, text) }
func (p palette) hint(text string) string  { return p.wrap(ansiHint, text) }

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

// render writes a human rendering for command and reports whether it produced
// one. A command without a rendering falls back to the indented JSON the caller
// already emits, so adding renderings is incremental.
func render(w io.Writer, p palette, command string, data any) bool {
	fields, ok := data.(map[string]any)
	if !ok {
		return false
	}
	switch command {
	case "gproxy status":
		return renderStatus(w, p, fields)
	case "gproxy server status":
		return renderServiceTable(w, p, fields)
	case "gproxy protocol show":
		return renderCatalogue(w, p, fields)
	}
	return false
}

func renderServiceTable(w io.Writer, p palette, fields map[string]any) bool {
	states, ok := fields["services"].([]service.Status)
	if !ok {
		return false
	}
	width := 0
	for _, state := range states {
		if len(state.Name) > width {
			width = len(state.Name)
		}
	}
	for _, state := range states {
		detail := p.hint("not installed")
		switch {
		case !state.Installed:
		case state.Running:
			detail = p.ok("running")
		default:
			detail = p.bad(clean(stateText(state.State)))
		}
		enabled := ""
		if state.Installed && !state.Enabled {
			enabled = gap + p.hint("not enabled at boot")
		}
		fmt.Fprintf(w, "  %s  %s%s\n", p.label(pad(clean(string(state.Name)), width)), detail, enabled)
	}
	return true
}

func renderCatalogue(w io.Writer, p palette, fields map[string]any) bool {
	entries, ok := fields["protocols"].([]map[string]string)
	if !ok {
		return false
	}
	width := 0
	for _, entry := range entries {
		if len(entry["name"]) > width {
			width = len(entry["name"])
		}
	}
	for _, entry := range entries {
		fmt.Fprintf(w, "  %s  %s\n", p.sys(pad(entry["name"], width)), entry["summary"])
	}
	return true
}

func renderStatus(w io.Writer, p palette, fields map[string]any) bool {
	rows := make([][2]string, 0, 6)
	if system, ok := fields["system"].(map[string]any); ok {
		rows = append(rows, [2]string{"system", p.sys(text(system["os"])) + p.hint(" · ") + p.sys(text(system["arch"]))})
	}
	if observed, ok := fields["network"].(network.Observation); ok {
		rows = append(rows, [2]string{"network", renderStack(p, observed)})
	}
	if _, ok := fields["nodes"]; ok {
		rows = append(rows, [2]string{"protocol", fmt.Sprintf("%s nodes%s%s users%s%s routes",
			p.count(text(fields["nodes"])), gap, p.count(text(fields["users"])), gap, p.count(text(fields["routing_rules"])))})
	}
	if states, ok := fields["services"].([]service.Status); ok && len(states) > 0 {
		rows = append(rows, [2]string{"services", renderServices(p, states)})
	}
	if certificate, ok := fields["cert"].(cert.Status); ok && certificate.Domain != "" {
		rows = append(rows, [2]string{"cert", renderCert(p, certificate)})
	}
	for _, issue := range issues(fields) {
		rows = append(rows, [2]string{"issue", p.bad(clean(issue))})
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "  %s  %s\n", p.label(pad(row[0], 8)), row[1]); err != nil {
			return true
		}
	}
	return true
}

// gap separates fields inside one row. Wide enough to read as a column break
// without aligning to a width the data may exceed.
const gap = "    "

func renderStack(p palette, observed network.Observation) string {
	var four, six bool
	for _, address := range observed.Addresses {
		if address.Scope == "loopback" || address.Scope == "link_local" {
			continue
		}
		switch address.Family {
		case "ipv4":
			four = true
		case "ipv6":
			six = true
		}
	}
	return "IPv4 " + available(p, four) + gap + "IPv6 " + available(p, six)
}

func available(p palette, present bool) string {
	if present {
		return p.ok("ok")
	}
	return p.bad("none")
}

func renderServices(p palette, states []service.Status) string {
	parts := make([]string, 0, len(states))
	for _, state := range states {
		switch {
		case !state.Installed:
			parts = append(parts, clean(string(state.Name))+" "+p.hint("absent"))
		case state.Running:
			parts = append(parts, clean(string(state.Name))+" "+p.ok("running"))
		default:
			parts = append(parts, clean(string(state.Name))+" "+p.bad(clean(stateText(state.State))))
		}
	}
	return strings.Join(parts, gap)
}

func stateText(state string) string {
	if state == "" {
		return "stopped"
	}
	return state
}

func renderCert(p palette, certificate cert.Status) string {
	if !certificate.Ready {
		return p.sys(clean(certificate.Domain)) + gap + p.bad("not issued")
	}
	if certificate.ExpiresAt == nil {
		return p.sys(clean(certificate.Domain)) + gap + p.ok("ready")
	}
	left := time.Until(*certificate.ExpiresAt)
	if left < 0 {
		return p.sys(clean(certificate.Domain)) + gap + p.bad("expired")
	}
	if left < 24*time.Hour {
		return p.sys(clean(certificate.Domain)) + gap + p.bad("expires in under a day")
	}
	// Rounded, not truncated: a certificate 89 days out reported as 88 reads as
	// an error in the display rather than as the deliberate margin it is not.
	days := int(math.Round(left.Hours() / 24))
	remaining := fmt.Sprintf("expires in %d days", days)
	if days == 1 {
		remaining = "expires in 1 day"
	}
	if days <= 14 {
		return p.sys(clean(certificate.Domain)) + gap + p.bad(remaining)
	}
	return p.sys(clean(certificate.Domain)) + gap + p.ok(remaining)
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

// clean removes C0 control characters from values that originate in systemd
// output, configuration or an error message. Without it a crafted unit name or
// issue text could inject an escape sequence into a rendering that reports
// itself as colour-free, which is exactly the guarantee the --no-color and
// non-terminal paths make.
func clean(value string) string {
	if strings.IndexFunc(value, control) < 0 {
		return value
	}
	return strings.Map(func(r rune) rune {
		if control(r) {
			return -1
		}
		return r
	}, value)
}

func control(r rune) bool { return r < 0x20 || r == 0x7f }

func pad(label string, width int) string {
	if len(label) >= width {
		return label
	}
	return label + strings.Repeat(" ", width-len(label))
}
