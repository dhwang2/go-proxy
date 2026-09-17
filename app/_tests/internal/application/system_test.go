package application

import (
	"go-proxy/internal/config"
	"path/filepath"
	"strings"
	"testing"
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
