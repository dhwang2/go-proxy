package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHDJailConfigUsesOneDayBan(t *testing.T) {
	if !strings.Contains(sshdJailConfig, "bantime = 86400") {
		t.Fatalf("sshdJailConfig = %q, want bantime = 86400", sshdJailConfig)
	}
}

// The files that switch the sshd jail on are read in fail2ban's order, the
// last setting wins, [DEFAULT] is not [sshd], and gproxy's own file is left
// to Managed. On Debian the package's own default keeps the jail on after
// gproxy's jail is removed, which is what the status has to be able to say.
func TestSSHDJailSourcesFollowFail2bansReadOrder(t *testing.T) {
	write := func(dir, name, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	debian := t.TempDir()
	write(debian, "jail.conf", "[DEFAULT]\nenabled = false\n\n[sshd]\nport = ssh\n")
	write(debian, "jail.d/defaults-debian.conf", "[sshd]\nenabled = true\n")
	write(debian, "jail.d/go-proxy-sshd.local", "[sshd]\nenabled = true\n")
	write(debian, "jail.d/sshd.local", "# shell-proxy\n[sshd]\nenabled=true\nmaxretry = 5\n")
	if got := strings.Join(sshdJailSources(debian), " "); got != "jail.d/defaults-debian.conf jail.d/sshd.local" {
		t.Fatalf("sources = %q", got)
	}

	// jail.local switches it off, discarding the .conf before it; a
	// jail.d/*.local read after it switches it on again.
	overridden := t.TempDir()
	write(overridden, "jail.d/defaults-debian.conf", "[sshd]\nenabled = true\n")
	write(overridden, "jail.local", "[sshd]\nenabled: no\n")
	if got := sshdJailSources(overridden); len(got) != 0 {
		t.Fatalf("a disabled jail still has sources %v", got)
	}
	write(overridden, "jail.d/zz.local", "[sshd]\nenabled = yes\n")
	if got := strings.Join(sshdJailSources(overridden), " "); got != "jail.d/zz.local" {
		t.Fatalf("sources = %q", got)
	}
}
