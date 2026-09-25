package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"go-proxy/internal/application"
	"go-proxy/internal/routing"
)

func complete(t *testing.T, args ...string) (candidates []string, directive string, lockDir string) {
	t.Helper()
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.LockDir = filepath.Join(t.TempDir(), "absent")
	code := r.Run(context.Background(), append([]string{"__complete"}, args...))
	if code != 0 {
		t.Fatalf("__complete %v exited %d: %s", args, code, stderr.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.HasPrefix(line, ":") {
			directive = line
			continue
		}
		if line == "" {
			continue
		}
		candidates = append(candidates, strings.SplitN(line, "\t", 2)[0])
	}
	return candidates, directive, r.App.LockDir
}

// noFileComp is cobra's ShellCompDirectiveNoFileComp. Anything that is not a
// path must carry it, or the shell silently offers file names for a value like
// an outbound tag or a congestion algorithm.
const noFileComp = ":4"

func TestClosedValueSetsComplete(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"protocol", "add", ""}, []string{"vless", "tuic", "anytls", "snell"}},
		{[]string{"config", "view", ""}, application.ConfigKinds()},
		{[]string{"server", "restart", ""}, application.ManagedServiceNames()},
		{[]string{"log", ""}, application.ManagedServiceNames()},
		{[]string{"route", "direct", "set", ""}, application.DirectStrategies()},
		{[]string{"network", "firewall", "add", "8443"}, []string{"8443/tcp", "8443/udp", "8443/both"}},
		{[]string{"network", "firewall", "remove", "8443/"}, []string{"8443/tcp", "8443/udp", "8443/both"}},
		{[]string{"protocol", "add", "--congestion", ""}, []string{"bbr", "cubic"}},
		{[]string{"route", "rule", "add", "--out", ""}, []string{"direct"}},
	}
	for _, testCase := range cases {
		t.Run(strings.Join(testCase.args, " "), func(t *testing.T) {
			got, directive, _ := complete(t, testCase.args...)
			if strings.Join(got, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("candidates %v, want %v", got, testCase.want)
			}
			if directive != noFileComp {
				t.Fatalf("directive %q, want %q — the shell would offer file names", directive, noFileComp)
			}
		})
	}
}

// rule add --rules takes menu indexes, so completion offers exactly those, never a
// preset name.
func TestPresetCompletionOffersTheMenuIndexes(t *testing.T) {
	got, directive, _ := complete(t, "route", "rule", "add", "--rules", "")
	if directive != noFileComp {
		t.Fatalf("directive %q, want %q", directive, noFileComp)
	}
	want := []string{}
	for _, choice := range routing.PresetMenu() {
		want = append(want, choice.Symbol)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("offered %v, want the menu indexes %v", got, want)
	}
}

// Free-form values are not paths. Offering file names for --user or --domain is
// worse than offering nothing.
func TestFreeFormFlagsDoNotCompleteFileNames(t *testing.T) {
	for _, args := range [][]string{
		{"protocol", "add", "--user", ""},
		{"protocol", "add", "--domain", ""},
		{"protocol", "add", "--port", ""},
		{"route", "test", "--user", ""},
		{"sub", "--target", ""},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			candidates, directive, _ := complete(t, args...)
			if directive != noFileComp {
				t.Fatalf("directive %q, want %q", directive, noFileComp)
			}
			if len(candidates) != 0 {
				t.Fatalf("free-form flag offered %v", candidates)
			}
		})
	}
}

// --parameter is a free-form endpoint, not a file.
func TestChainParameterOffersNoFiles(t *testing.T) {
	_, directive, _ := complete(t, "route", "chain", "add", "relay", "--parameter", "")
	if directive != noFileComp {
		t.Fatal("--parameter offered file names")
	}
}

// Completion runs on every keystroke a person presses Tab. It must cost what
// help costs: no store load, no lock, no runtime directory.
func TestCompletionTouchesNoRuntimeState(t *testing.T) {
	_, _, lockDir := complete(t, "route", "")
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatal("completion created runtime state")
	}
}

// cobra adds the completion command during ExecuteC, which would place it
// after the groups walk and leave it exiting 0 with help on stdout even under
// --json. Root() adds it early instead; this pins that.
func TestCompletionGroupObeysTheUsageContract(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.LockDir = filepath.Join(t.TempDir(), "absent")
	if code := r.Run(context.Background(), []string{"completion", "--json"}); code != 2 {
		t.Fatalf("bare completion exited %d, want 2; stdout=%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"code":"invalid_argument"`) {
		t.Fatalf("bare completion did not return the usual failure envelope: %s", out.String())
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Fatalf("help text reached stdout: %s", out.String())
	}
}

func TestCompletionGeneratorIsAvailable(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.LockDir = filepath.Join(t.TempDir(), "absent")
	if code := r.Run(context.Background(), []string{"completion", "zsh"}); code != 0 {
		t.Fatalf("completion zsh exited %d: %s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "compdef") {
		t.Fatalf("zsh completion script looks wrong: %.120s", out.String())
	}
}

// Tab lists bare names: every shell's script asks for completions without
// descriptions, and there is no --no-descriptions flag to turn them back on.
func TestCompletionListsNamesOnly(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		var out, stderr bytes.Buffer
		r := New("test", "test", strings.NewReader(""), &out, &stderr)
		if code := r.Run(context.Background(), []string{"completion", shell}); code != 0 {
			t.Fatalf("%s: exit %d: %s", shell, code, stderr.String())
		}
		if !strings.Contains(out.String(), "__completeNoDesc") {
			t.Fatalf("%s: the script asks for descriptions", shell)
		}
	}
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	if code := r.Run(context.Background(), []string{"completion", "bash", "--no-descriptions"}); code != 2 {
		t.Fatalf("--no-descriptions still accepted: exit %d", code)
	}
}

// Nothing gproxy takes is a file, so Tab never falls back to file names:
// after a command with no arguments, in a free-form argument and in every
// value flag, the answer is nothing, with file completion switched off.
func TestTabNeverOffersFiles(t *testing.T) {
	for _, args := range [][]string{
		{"status", "."}, {"status", ""}, {"version", ""}, {"user", "add", ""}, {"route", "chain", "add", ""},
		{"sub", ""}, {"route", "rule", "add", "--user", ""}, {"status", "--timeout", ""}, {"log", "--lines", ""},
	} {
		got, directive, _ := complete(t, args...)
		if len(got) != 0 || directive != noFileComp {
			t.Fatalf("%v: offered %v with directive %q, want nothing and %q", args, got, directive, noFileComp)
		}
	}
}

// No position anywhere in the command tree falls back to file names: for
// every command, at the first argument and after one, and for every flag
// that takes a value, the answer carries NoFileComp.
func TestNoCompletionFallsBackToFiles(t *testing.T) {
	type position struct{ args []string }
	var positions []position
	var walk func(cmd *cobra.Command, path []string)
	walk = func(cmd *cobra.Command, path []string) {
		for _, tail := range [][]string{{""}, {"."}, {"x", ""}, {"x", "."}, {"x", "y", "."}} {
			positions = append(positions, position{append(append([]string(nil), path...), tail...)})
		}
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Value.Type() != "bool" {
				positions = append(positions, position{append(append([]string(nil), path...), "--"+flag.Name, ".")})
			}
		})
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() {
				walk(child, append(append([]string(nil), path...), child.Name()))
			}
		}
	}
	walk(New("test", "test", strings.NewReader(""), io.Discard, io.Discard).Root(), nil)
	for _, at := range positions {
		_, directive, _ := complete(t, at.args...)
		value, err := strconv.Atoi(strings.TrimPrefix(directive, ":"))
		if err != nil || value&4 == 0 {
			t.Fatalf("gproxy %s<Tab> may offer file names (directive %q)", strings.Join(at.args, " "), directive)
		}
	}
	t.Logf("checked %d completion positions", len(positions))
}
