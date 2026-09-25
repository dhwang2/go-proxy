package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/core"
)

func TestSystemSelectorsRequireUnambiguousManagedScope(t *testing.T) {
	for _, test := range []struct {
		selector string
		all      bool
	}{{"", false}, {"sshd", false}, {"sing-box", true}} {
		if _, err := ManagedServices(test.selector, test.all); err == nil {
			t.Fatalf("accepted service scope %+v", test)
		}
	}
	names, err := ManagedServices("sing-box", false)
	if err != nil || len(names) != 1 || names[0] != "sing-box" {
		t.Fatalf("valid service rejected %v %v", names, err)
	}
	for _, test := range []struct {
		selector string
		all      bool
	}{{"", false}, {"singbox", false}, {"sing-box", true}} {
		if _, err := Components(test.selector, test.all); err == nil {
			t.Fatalf("accepted core scope %+v", test)
		}
	}
	cores, err := Components("snell", false)
	if err != nil || len(cores) != 1 || cores[0] != "snell" {
		t.Fatalf("valid core rejected %v %v", cores, err)
	}
}

func TestBootRulesRestoreStateLockWithoutTruncation(t *testing.T) {
	rules := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(lockSetupRules), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 6 {
			t.Fatalf("invalid tmpfiles rule: %q", line)
		}
		if fields[3] != "root" || fields[4] != "root" || fields[5] != "-" {
			t.Fatalf("unsafe lock owner or expiry: %q", line)
		}
		rules[fields[1]] = fields
	}
	dirRule := rules[config.LockDir]
	if len(dirRule) == 0 || dirRule[0] != "d" || dirRule[2] != "0755" {
		t.Fatal("boot rules do not create the lock directory")
	}
	stateRule := rules[filepath.Join(config.LockDir, ".state.lock")]
	if len(stateRule) == 0 || stateRule[0] != "f" || stateRule[2] != "0600" {
		t.Fatal("boot rules must create a private state lock without truncating an existing file")
	}
	if len(rules) != 2 {
		t.Fatal("boot rules change unrelated paths")
	}
}

// Completion scripts written by the installer are owned resources. If they are
// absent from the inventory, uninstall leaves them behind as litter pointing at
// a binary that no longer exists; if the inventory named a directory instead of
// the two exact files, uninstall would take the shell's own completion tree
// with it.
func TestUninstallPreviewOwnsTheCompletionScripts(t *testing.T) {
	a := protocolTestApp(t)
	paths, err := a.uninstallScope()
	if err != nil {
		t.Fatal(err)
	}
	owned := map[string]bool{}
	for _, path := range paths {
		owned[path] = true
	}
	for _, want := range []string{config.BashCompletionPath, config.ZshCompletionPath} {
		if !owned[want] {
			t.Fatalf("completion script %q is not an owned resource: %v", want, paths)
		}
	}
	for _, unwanted := range []string{"/usr/share/bash-completion", "/usr/share/bash-completion/completions", "/usr/share/zsh", "/usr/share/zsh/site-functions"} {
		if owned[unwanted] {
			t.Fatalf("uninstall claims the shell's own directory %q", unwanted)
		}
	}
}

// One installed core, one version. `core version` read the executable while
// `core check` read the receipt beside it, so the two commands disagreed about
// the same snell install. The helper was not enough: this asserts the commands.
func TestCoreVersionAndCheckAgreeOnAnInstalledCore(t *testing.T) {
	dir := t.TempDir()
	saved := config.SnellBin
	config.SnellBin = filepath.Join(dir, "snell-server")
	t.Cleanup(func() { config.SnellBin = saved })
	if err := os.WriteFile(config.SnellBin, []byte("#!/bin/sh\nprintf '%s\\n' 'snell-server v6.0.0' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SnellBin+".version", []byte(core.SnellVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(nil)
	a.RequireRoot = false
	ctx := context.Background()

	versions, err := a.CoreVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reported := ""
	infos, _ := versions.Data.([]core.VersionInfo)
	for _, info := range infos {
		if info.Component == core.CompSnell {
			reported = info.Version
		}
	}
	if reported != core.SnellVersion {
		t.Fatalf("core version reported %q, want the archive version %q", reported, core.SnellVersion)
	}

	checked, err := a.CoreCheck(ctx, "snell")
	if err != nil {
		t.Fatal(err)
	}
	checks, _ := checked.Data.([]*core.UpdateCheck)
	if len(checks) != 1 {
		t.Fatalf("core check returned %d results", len(checks))
	}
	if checks[0].CurrentVersion != reported {
		t.Fatalf("core check reports %q where core version reports %q", checks[0].CurrentVersion, reported)
	}
	if checks[0].UpdateAvail {
		t.Fatalf("the installed archive was reported as updatable: %+v", checks[0])
	}
}

// Uninstall takes the installer's completion block out of /etc/bash.bashrc and
// leaves every other line as it was; a file without the block, a half block, or
// no file at all is left alone.
func TestRemoveMarkedBlockTakesOnlyTheBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bash.bashrc")
	begin, end := config.BashrcCompletionBeginMark, config.BashrcCompletionEndMark
	original := "# system-wide .bashrc\nPS1='\\u@\\h:\\w\\$ '\n"
	withBlock := original + begin + "\nif true; then\n  . /usr/share/bash-completion/bash_completion\nfi\n" + end + "\nalias ll='ls -l'\n"
	if err := os.WriteFile(path, []byte(withBlock), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeMarkedBlock(path, begin, end); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := original + "alias ll='ls -l'\n"; string(got) != want {
		t.Fatalf("after removal:\n%q\nwant\n%q", got, want)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Fatalf("mode changed to %v", info.Mode().Perm())
	}
	half := original + begin + "\nno end marker\n"
	if err := os.WriteFile(path, []byte(half), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeMarkedBlock(path, begin, end); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != half {
		t.Fatalf("a half block was edited: %q", got)
	}
	if err := removeMarkedBlock(filepath.Join(dir, "absent"), begin, end); err != nil {
		t.Fatalf("an absent file is an error: %v", err)
	}
}
