package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"go-proxy/internal/config"
)

// groupsWithoutDefaultAction lists every command group that has no default
// action. Invoking one without a subcommand is a missing-argument usage error.
var groupsWithoutDefaultAction = [][]string{
	{"cert"},
	{"config"},
	{"network"},
	{"network", "bbr"},
	{"network", "fail2ban"},
	{"network", "firewall"},
	{"completion"},
	{"route"},
	{"user"},
	{"core"},
	{"route", "chain"},
	{"route", "direct"},
	{"route", "final"},
	{"route", "rule"},
	{"protocol"},
	{"server"},
}

func groupTestRunner(t *testing.T, out, stderr io.Writer) *Runner {
	t.Helper()
	r := New("test", "test", strings.NewReader(""), out, stderr)
	r.App.RequireRoot = false
	r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
	return r
}

func decodeSingleEnvelope(t *testing.T, data []byte) struct {
	OK      bool `json:"ok"`
	Changed bool `json:"changed"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
} {
	t.Helper()
	var envelope struct {
		OK      bool `json:"ok"`
		Changed bool `json:"changed"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v: %s", err, data)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout must contain exactly one JSON value: %v: %s", err, data)
	}
	return envelope
}

func TestGroupWithoutSubcommandIsJSONUsageError(t *testing.T) {
	for _, group := range groupsWithoutDefaultAction {
		t.Run(strings.Join(group, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r := groupTestRunner(t, &stdout, &stderr)
			if code := r.Run(context.Background(), append(append([]string(nil), group...), "--json")); code != 2 {
				t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			envelope := decodeSingleEnvelope(t, stdout.Bytes())
			if envelope.OK || envelope.Changed {
				t.Fatalf("wrong envelope state: %s", stdout.String())
			}
			if envelope.Error.Code != "invalid_argument" {
				t.Fatalf("error code %q, want invalid_argument", envelope.Error.Code)
			}
			if !strings.Contains(envelope.Error.Message, "gproxy "+strings.Join(group, " ")) {
				t.Fatalf("message does not name the command: %q", envelope.Error.Message)
			}
			for _, name := range subcommandNames(t, group) {
				if !strings.Contains(envelope.Error.Message, name) {
					t.Fatalf("message omits subcommand %q: %q", name, envelope.Error.Message)
				}
			}
			for _, fragment := range []string{"Usage:", "Available Commands:", "Flags:"} {
				if strings.Contains(stdout.String(), fragment) {
					t.Fatalf("help text reached stdout: %s", stdout.String())
				}
			}
			if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
				t.Fatal("usage error initialized runtime state")
			}
		})
	}
}

func TestGroupWithoutSubcommandIsHumanUsageError(t *testing.T) {
	for _, group := range groupsWithoutDefaultAction {
		t.Run(strings.Join(group, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r := groupTestRunner(t, &stdout, &stderr)
			if code := r.Run(context.Background(), group); code != 2 {
				t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("usage error wrote to stdout: %s", stdout.String())
			}
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				t.Fatal("usage error produced no diagnostic")
			}
			if strings.Count(message, "requires a subcommand") != 1 {
				t.Fatalf("duplicate or missing error report: %s", message)
			}
			if strings.HasSuffix(message, ".") {
				t.Fatalf("error ends with punctuation: %s", message)
			}
			if message != strings.ToLower(message) {
				t.Fatalf("error is not lowercase: %s", message)
			}
		})
	}
}

func TestGroupHelpStaysHumanAndSucceeds(t *testing.T) {
	for _, group := range groupsWithoutDefaultAction {
		for _, extra := range [][]string{{"--help"}, {"--json", "--help"}} {
			args := append(append([]string(nil), group...), extra...)
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				r := groupTestRunner(t, &stdout, &stderr)
				if code := r.Run(context.Background(), args); code != 0 {
					t.Fatalf("exit %d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
				}
				if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "gproxy "+strings.Join(group, " ")) {
					t.Fatalf("help is missing: %s", stdout.String())
				}
				if json.Valid(bytes.TrimSpace(stdout.Bytes())) {
					t.Fatalf("help emitted JSON: %s", stdout.String())
				}
				if stderr.Len() != 0 {
					t.Fatalf("help emitted diagnostics: %s", stderr.String())
				}
				if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
					t.Fatal("help initialized runtime state")
				}
			})
		}
	}
}

// TestGroupClassificationMatchesCommandTree walks the real command tree so a
// newly added group cannot silently fall back to printing help on stdout.
func TestGroupClassificationMatchesCommandTree(t *testing.T) {
	var found []string
	for _, path := range commandsWithSubcommands(t) {
		var stdout, stderr bytes.Buffer
		r := groupTestRunner(t, &stdout, &stderr)
		code := r.Run(context.Background(), append(append([]string(nil), path...), "--json"))
		if strings.Contains(stdout.String(), "Usage:") {
			t.Fatalf("%s wrote help to stdout", strings.Join(path, " "))
		}
		envelope := decodeSingleEnvelope(t, stdout.Bytes())
		if code == 2 && envelope.Error.Code == "invalid_argument" {
			found = append(found, strings.Join(path, " "))
		}
	}
	want := make([]string, 0, len(groupsWithoutDefaultAction))
	for _, group := range groupsWithoutDefaultAction {
		want = append(want, strings.Join(group, " "))
	}
	sort.Strings(found)
	sort.Strings(want)
	if strings.Join(found, "|") != strings.Join(want, "|") {
		t.Fatalf("group commands without a default action are %v, want %v", found, want)
	}
}

// Every group is now a pure namespace: naming one without a subcommand is a
// usage error, never a silent default action. A group that quietly did
// something would make `gproxy protocol` and `gproxy protocol list` differ for
// no visible reason.
func TestNoGroupCommandHasADefaultAction(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		children := []*cobra.Command{}
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() {
				children = append(children, child)
			}
		}
		if len(children) > 0 && len(path) > 0 {
			var stdout, errors bytes.Buffer
			runner := New("test", "test", strings.NewReader(""), &stdout, &errors)
			runner.App.RequireRoot = false
			runner.App.LockDir = filepath.Join(t.TempDir(), "absent")
			if code := runner.Run(context.Background(), append(append([]string(nil), path...), "--json")); code != 2 {
				t.Fatalf("%s acted without a subcommand: exit=%d stdout=%s", strings.Join(path, " "), code, stdout.String())
			}
		}
		for _, child := range children {
			walk(child, append(append([]string(nil), path...), child.Name()))
		}
	}
	walk(r.Root(), nil)
}

func subcommandNames(t *testing.T, path []string) []string {
	t.Helper()
	cmd := findCommand(t, path)
	names := []string{}
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			names = append(names, child.Name())
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s has no available subcommands", strings.Join(path, " "))
	}
	return names
}

func findCommand(t *testing.T, path []string) *cobra.Command {
	t.Helper()
	cmd := New("test", "test", strings.NewReader(""), io.Discard, io.Discard).Root()
	for _, name := range path {
		next, _, err := cmd.Find([]string{name})
		if err != nil || next == cmd {
			t.Fatalf("command %s not found", strings.Join(path, " "))
		}
		cmd = next
	}
	return cmd
}

func commandsWithSubcommands(t *testing.T) [][]string {
	t.Helper()
	var paths [][]string
	var walk func(*cobra.Command, []string)
	walk = func(cmd *cobra.Command, path []string) {
		if len(path) > 0 && cmd.HasSubCommands() {
			paths = append(paths, path)
		}
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() {
				walk(child, append(append([]string(nil), path...), child.Name()))
			}
		}
	}
	walk(New("test", "test", strings.NewReader(""), io.Discard, io.Discard).Root(), nil)
	return paths
}

// initializedRuntimeFixture points the managed configuration paths at a
// test-owned directory so default group actions can complete successfully.
func initializedRuntimeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	for target, name := range map[*string]string{
		&config.SingBoxConfig:      "conf/sing-box.json",
		&config.UserMetaFile:       "user-management.json",
		&config.UserRouteFile:      "user-route-rules.json",
		&config.UserTemplateFile:   "user-route-templates.json",
		&config.FirewallConfigFile: "firewall-ports.json",
		&config.SnellConfigFile:    "snell-v6.conf",
	} {
		original := *target
		*target = filepath.Join(dir, filepath.FromSlash(name))
		t.Cleanup(func() { *target = original })
	}
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{"inbounds":[],"outbounds":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lockDir := filepath.Join(dir, "lock")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, ".state.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return lockDir
}

// Every help page lists commands as "name (what it does)" and flags as
// "--flag type (what it does)": each explanation a lowercase note one space
// after what it explains, and no blank line between the sections.
func TestHelpPutsExplanationsInLowercaseNotes(t *testing.T) {
	var paths [][]string
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		paths = append(paths, path)
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() {
				walk(child, append(append([]string(nil), path...), child.Name()))
			}
		}
	}
	walk(New("test", "test", strings.NewReader(""), io.Discard, io.Discard).Root(), nil)
	unspaced := regexp.MustCompile(`[^\s(\[<]\(`)
	for _, path := range paths {
		var out, stderr bytes.Buffer
		r := New("test", "test", strings.NewReader(""), &out, &stderr)
		if code := r.Run(context.Background(), append(append([]string(nil), path...), "--help")); code != 0 {
			t.Fatalf("%v --help: exit %d: %s", path, code, stderr.String())
		}
		text := out.String()
		if strings.Contains(text, "\n\n") {
			t.Fatalf("%v --help has a blank line:\n%s", path, text)
		}
		section := ""
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ") {
				section = line
				continue
			}
			if section != "Available Commands:" && section != "Flags:" && section != "Global Flags:" || strings.HasPrefix(line, "Use ") {
				continue
			}
			open := strings.Index(line, " (")
			if open < 0 || !strings.HasSuffix(line, ")") || unspaced.MatchString(line) {
				t.Fatalf("%v --help: %q is not \"name (note)\"", path, line)
			}
			if inside := line[open+2 : len(line)-1]; inside != strings.ToLower(inside) {
				t.Fatalf("%v --help: uppercase in %q", path, line)
			}
		}
	}
}
