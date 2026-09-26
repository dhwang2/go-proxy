package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/network"
)

// The banned list is rewritten whole on every report, one address per line,
// and emptied when nothing is banned so it never shows a stale list. A host
// without fail2ban gets no file.
func TestBannedAddressesAreWrittenOnePerLine(t *testing.T) {
	saved := config.Fail2BanBannedFile
	config.Fail2BanBannedFile = filepath.Join(t.TempDir(), "fail2ban-banned.txt")
	t.Cleanup(func() { config.Fail2BanBannedFile = saved })

	info := network.Fail2BanInfo{Installed: true, BannedIPs: []string{"198.51.100.5", "2001:db8::7"}}
	recordBanned(&info)
	if info.BannedFile != config.Fail2BanBannedFile {
		t.Fatalf("banned_file = %q", info.BannedFile)
	}
	if got, _ := os.ReadFile(config.Fail2BanBannedFile); string(got) != "198.51.100.5\n2001:db8::7\n" {
		t.Fatalf("file = %q", got)
	}

	info = network.Fail2BanInfo{Installed: true, BannedIPs: []string{}}
	recordBanned(&info)
	if got, err := os.ReadFile(config.Fail2BanBannedFile); err != nil || len(got) != 0 {
		t.Fatalf("an empty list left %q (%v)", got, err)
	}

	config.Fail2BanBannedFile = filepath.Join(t.TempDir(), "absent", "fail2ban-banned.txt")
	info = network.Fail2BanInfo{Installed: false}
	recordBanned(&info)
	if _, err := os.Stat(config.Fail2BanBannedFile); !os.IsNotExist(err) || info.BannedFile != "" {
		t.Fatalf("a host without fail2ban got a file: %v %q", err, info.BannedFile)
	}
}

// disable stops fail2ban itself and reports the bans that went with it; a
// second disable finds it already stopped and changes nothing.
func TestFail2banDisableStopsTheService(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	stubs := t.TempDir()
	savedBanned, savedDir, savedJail := config.Fail2BanBannedFile, network.Fail2BanConfigDir, network.Fail2BanJailPath
	config.Fail2BanBannedFile = filepath.Join(stubs, "banned.txt")
	network.Fail2BanConfigDir = filepath.Join(stubs, "fail2ban")
	network.Fail2BanJailPath = filepath.Join(stubs, "fail2ban", "jail.d", "go-proxy-sshd.local")
	t.Cleanup(func() {
		config.Fail2BanBannedFile, network.Fail2BanConfigDir, network.Fail2BanJailPath = savedBanned, savedDir, savedJail
	})
	if err := os.MkdirAll(filepath.Dir(network.Fail2BanJailPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Builtins only: PATH holds nothing but these stubs. The state file says
	// whether the service is running.
	scripts := map[string]string{
		"systemctl": `state="${0%/*}/state"; read s < "$state"
case "$1" in
show) if [ "$s" = on ]; then printf 'LoadState=loaded\nActiveState=active\nUnitFileState=enabled\n'; else printf 'LoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n'; fi ;;
disable) echo off > "$state" ;;
enable) [ "$s" = on ] || { echo on > "$state"; echo 2 > "${0%/*}/starting"; } ;;
esac`,
		// A service just started answers nothing for its first two queries,
		// as fail2ban does until its socket is up.
		"fail2ban-client": `starting="${0%/*}/starting"; read n < "$starting"
if [ "$n" -gt 0 ]; then echo $((n - 1)) > "$starting"; echo "ERROR   Could not find server" >&2; exit 255; fi
case "$1 $2 $3" in
"status  ") printf 'Number of jail:\t1\n` + "`" + `- Jail list:\tsshd\n' ;;
"status sshd ") printf '|- Currently banned:\t2\n|- Total banned:\t5\n` + "`" + `- Banned IP list:\t198.51.100.1 198.51.100.2\n' ;;
*) echo 5 ;;
esac`,
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(stubs, "state"), []byte("on\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubs, "starting"), []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubs)

	result, err := a.NetworkFail2Ban(ctx, "disable")
	if err != nil {
		t.Fatal(err)
	}
	info := result.Data.(network.Fail2BanInfo)
	if !result.Changed || info.Change != "stopped" || info.Running || info.BansLifted != 2 {
		t.Fatalf("disable: changed=%v %#v", result.Changed, info)
	}
	again, err := a.NetworkFail2Ban(ctx, "disable")
	if err != nil {
		t.Fatal(err)
	}
	if info := again.Data.(network.Fail2BanInfo); again.Changed || info.Change != "already stopped" {
		t.Fatalf("second disable: changed=%v %#v", again.Changed, info)
	}

	// enable from stopped: no reload into a server that is not up yet, and
	// the jail is waited for rather than reported missing.
	started, err := a.NetworkFail2Ban(ctx, "enable")
	if err != nil {
		t.Fatal(err)
	}
	if info := started.Data.(network.Fail2BanInfo); !started.Changed || info.Change != "started" || !info.Running || !info.SSHJailEnabled || !info.Managed {
		t.Fatalf("enable: changed=%v %#v", started.Changed, info)
	}
}

// enable persists BBR in gproxy's file; disable switches the kernel back to
// cubic, removes that file and names another file that re-enables BBR at
// boot; a second disable is a no-op.
func TestBBRDisableRevertsTheKernelAndNamesBootSources(t *testing.T) {
	a := routingApplicationFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	proc := filepath.Join(dir, "tcp_congestion_control")
	etc := filepath.Join(dir, "sysctl.d")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := []any{network.CongestionControlPath, network.BBRSysctlPath, network.SysctlDirs, network.SysctlConf}
	network.CongestionControlPath, network.BBRSysctlPath = proc, filepath.Join(etc, "90-go-proxy-bbr.conf")
	network.SysctlDirs, network.SysctlConf = []string{etc}, filepath.Join(dir, "sysctl.conf")
	t.Cleanup(func() {
		network.CongestionControlPath, network.BBRSysctlPath = saved[0].(string), saved[1].(string)
		network.SysctlDirs, network.SysctlConf = saved[2].([]string), saved[3].(string)
	})
	// Builtins only: PATH holds nothing but these stubs.
	stub := "#!/bin/sh\ncase \"$2\" in net.ipv4.tcp_congestion_control=*) echo \"${2#*=}\" > '" + proc + "' ;; esac\nexit 0\n"
	for name, body := range map[string]string{"sysctl": stub, "modprobe": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	if err := os.WriteFile(proc, []byte("cubic\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := a.NetworkBBR(ctx, "enable")
	if data := result.Data.(map[string]any); err != nil || !result.Changed || data["enabled"] != true || data["managed"] != true || data["change"] != "enabled" {
		t.Fatalf("enable: %#v %v", result, err)
	}
	// Another file keeps BBR at boot, as a shell-proxy leftover would.
	if err := os.WriteFile(filepath.Join(etc, "99-bbr-proxy.conf"), []byte("net.ipv4.tcp_congestion_control = bbr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = a.NetworkBBR(ctx, "disable")
	data := result.Data.(map[string]any)
	if err != nil || !result.Changed || data["enabled"] != false || data["current"] != "cubic" || data["managed"] != false || data["change"] != "disabled" {
		t.Fatalf("disable: %#v %v", result, err)
	}
	if sources, _ := data["boot_sources"].([]string); len(sources) != 1 || filepath.Base(sources[0]) != "99-bbr-proxy.conf" {
		t.Fatalf("boot sources = %v", data["boot_sources"])
	}
	if _, err := os.Stat(network.BBRSysctlPath); !os.IsNotExist(err) {
		t.Fatal("gproxy's bbr file outlived disable")
	}
	again, err := a.NetworkBBR(ctx, "disable")
	if err != nil || again.Changed || again.Data.(map[string]any)["change"] != "already disabled" {
		t.Fatalf("second disable: %#v %v", again, err)
	}
	status, err := a.NetworkBBR(ctx, "status")
	if err != nil || status.Data.(map[string]any)["enabled"] != false {
		t.Fatalf("status: %#v %v", status, err)
	}
}
