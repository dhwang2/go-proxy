package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What keeps BBR at boot is read the way systemd-sysctl applies it: files by
// name across the directories, an earlier directory hiding a later file of the
// same name, /etc/sysctl.conf last, and the last setting of the key winning.
// gproxy's own file is reported separately, as managed.
func TestBBRBootSourcesFollowSysctlOrder(t *testing.T) {
	root := t.TempDir()
	etc, lib := filepath.Join(root, "etc"), filepath.Join(root, "lib")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved := []any{SysctlDirs, SysctlConf, BBRSysctlPath}
	SysctlDirs, SysctlConf = []string{etc, lib}, filepath.Join(root, "sysctl.conf")
	BBRSysctlPath = filepath.Join(etc, "90-go-proxy-bbr.conf")
	t.Cleanup(func() {
		SysctlDirs, SysctlConf, BBRSysctlPath = saved[0].([]string), saved[1].(string), saved[2].(string)
	})

	write(filepath.Join(lib, "50-default.conf"), "net.ipv4.tcp_congestion_control = cubic\n")
	write(BBRSysctlPath, "net.ipv4.tcp_congestion_control = bbr\n")
	write(filepath.Join(etc, "99-bbr-proxy.conf"), "# shell-proxy\nnet.core.default_qdisc = fq\nnet/ipv4/tcp_congestion_control=bbr\n")
	if got := strings.Join(BBRBootSources(), " "); got != filepath.Join(etc, "99-bbr-proxy.conf") {
		t.Fatalf("sources = %q", got)
	}

	// /etc/sysctl.conf is applied last and switches it back.
	write(SysctlConf, "net.ipv4.tcp_congestion_control = cubic\n")
	if got := BBRBootSources(); len(got) != 0 {
		t.Fatalf("a later cubic left sources %v", got)
	}

	// A file in /etc hides the one of the same name in a later directory.
	os.Remove(SysctlConf)
	write(filepath.Join(lib, "99-bbr-proxy.conf"), "net.ipv4.tcp_congestion_control = cubic\n")
	if got := strings.Join(BBRBootSources(), " "); got != filepath.Join(etc, "99-bbr-proxy.conf") {
		t.Fatalf("the /etc file did not hide the later one: %q", got)
	}
}
