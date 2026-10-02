package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/network"
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

// A fresh runtime turns BBR on and reports what the kernel runs afterwards:
// BBR already on is left alone, a kernel that switches says enabled, and one
// that refuses is reported without failing the install.
func TestDefaultBBRReportsWhatTheKernelRuns(t *testing.T) {
	dir := t.TempDir()
	savedCC, savedFile := network.CongestionControlPath, network.BBRSysctlPath
	network.CongestionControlPath = filepath.Join(dir, "tcp_congestion_control")
	network.BBRSysctlPath = filepath.Join(dir, "90-go-proxy-bbr.conf")
	t.Cleanup(func() { network.CongestionControlPath, network.BBRSysctlPath = savedCC, savedFile })
	t.Setenv("PATH", dir)
	t.Setenv("CC_FILE", network.CongestionControlPath)
	stub := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	stub("modprobe", "exit 0\n")
	a := New(func(string) {})
	for _, c := range []struct {
		kernel, sysctl, want string
	}{
		{"bbr", "exit 1\n", "already enabled"},
		{"cubic", "case \"$2\" in net.ipv4.tcp_congestion_control=*) echo \"${2#*=}\" > \"$CC_FILE\";; esac\n", "enabled"},
		{"cubic", "echo 'sysctl: setting key: No such file or directory' >&2; exit 1\n", "unavailable"},
		{"cubic", "exit 0\n", "unavailable: the kernel still uses cubic"},
	} {
		if err := os.WriteFile(network.CongestionControlPath, []byte(c.kernel+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		stub("sysctl", c.sysctl)
		if got := a.defaultBBR(context.Background()); !strings.HasPrefix(got, c.want) {
			t.Fatalf("kernel %s: got %q, want %q", c.kernel, got, c.want)
		}
	}
}

// systemd stops the watchdog by cancelling it. That is its normal end: an
// error here was logged as "error: operation cancelled" on every stop and
// recorded the unit as failed.
func TestWatchdogStopsCleanlyWhenCancelled(t *testing.T) {
	a := New(nil)
	a.RequireRoot = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Watchdog(ctx); err != nil {
		t.Fatalf("a cancelled watchdog returned %v, want a clean stop", err)
	}
}
