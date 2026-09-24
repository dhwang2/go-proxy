package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotBatchesAndDistinguishesMissing(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/bin/sh\nprintf '%s\\n' call >> \"$TEST_SERVICE_CALLS\"\nprintf '%s\\n' 'Id=sing-box.service' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=enabled' '' 'Id=snell-v6.service' 'LoadState=not-found' 'ActiveState=inactive' 'UnitFileState='\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_SERVICE_CALLS", calls)
	statuses, err := Snapshot(context.Background(), SingBox, Snell)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 || !statuses[0].Installed || !statuses[0].Running || statuses[1].Installed || statuses[1].State != "missing" {
		t.Fatalf("wrong states: %+v", statuses)
	}
	output, _ := os.ReadFile(calls)
	if strings.Count(string(output), "call") != 1 {
		t.Fatal("service observation was not batched")
	}
}

func TestWaitReadyObservesStableActiveAndEarlyFailure(t *testing.T) {
	for _, second := range []string{"active", "failed"} {
		t.Run(second, func(t *testing.T) {
			dir := t.TempDir()
			count := filepath.Join(dir, "count")
			script := "#!/bin/sh\nn=0\nif [ -f \"$TEST_READY_COUNT\" ]; then read n < \"$TEST_READY_COUNT\"; fi\nn=$((n+1))\nprintf '%s\\n' \"$n\" > \"$TEST_READY_COUNT\"\nstate=active\nif [ \"$n\" -gt 1 ]; then state=\"$TEST_READY_SECOND\"; fi\nprintf '%s\\n' 'Id=sing-box.service' 'LoadState=loaded' \"ActiveState=$state\" 'UnitFileState=enabled'\n"
			if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TEST_READY_COUNT", count)
			t.Setenv("TEST_READY_SECOND", second)
			err := WaitReady(context.Background(), SingBox)
			if (err == nil) != (second == "active") {
				t.Fatalf("wrong readiness result: %v", err)
			}
			data, _ := os.ReadFile(count)
			if strings.TrimSpace(string(data)) != "2" {
				t.Fatalf("readiness did not observe twice: %s", data)
			}
		})
	}
}

func TestWaitReadyHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' 'Id=sing-box.service' 'LoadState=loaded' 'ActiveState=activating' 'UnitFileState=enabled'\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	before := time.Now()
	if err := WaitReady(ctx, SingBox); err == nil {
		t.Fatal("cancelled readiness succeeded")
	}
	if time.Since(before) > time.Second {
		t.Fatal("readiness ignored cancellation")
	}
}

func TestSnapshotDoesNotReportManagerFailureAsMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit 1\n"), 0755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	statuses, err := Snapshot(context.Background(), SingBox)
	if err == nil {
		t.Fatal("manager failure returned success")
	}
	if statuses[0].State != "unknown" || statuses[0].Error == "" {
		t.Fatalf("lost observation failure: %+v", statuses)
	}
}

func TestManualStopGroupAndIndependentResume(t *testing.T) {
	oldStopped, oldUnits := stoppedDir, shadowTLSUnitDir
	stoppedDir = t.TempDir()
	shadowTLSUnitDir = t.TempDir()
	t.Cleanup(func() { stoppedDir = oldStopped; shadowTLSUnitDir = oldUnits })
	for _, name := range []string{"shadow-tls-ss-8388", "shadow-tls-snell-1443"} {
		if err := os.WriteFile(filepath.Join(shadowTLSUnitDir, name+".service"), []byte("unit"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := SetStopped(ShadowTLS, true); err != nil {
		t.Fatal(err)
	}
	first, second := Name("shadow-tls-ss-8388"), Name("shadow-tls-snell-1443")
	if !IsStopped(first) || !IsStopped(second) {
		t.Fatal("group stop did not stop every binding")
	}
	if err := SetStopped(first, false); err != nil {
		t.Fatal(err)
	}
	if IsStopped(first) || !IsStopped(second) {
		t.Fatal("individual resume changed a different binding")
	}
	if err := SetStopped(ShadowTLS, false); err != nil {
		t.Fatal(err)
	}
	if IsStopped(first) || IsStopped(second) || IsStopped(ShadowTLS) {
		t.Fatal("group resume left a stopped marker")
	}
}

// A reinstall replaces the binary under a running watchdog. The watchdog is
// stale when its executable is not this binary -- a replaced file shows as
// "<path> (deleted)" -- and not when it is, or when it is not running.
func TestWatchdogRunsStaleBinary(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	systemctl := "#!/bin/sh\ncat " + pidFile + "\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(systemctl), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	saved := procRoot
	procRoot = filepath.Join(dir, "proc")
	t.Cleanup(func() { procRoot = saved })
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	for name, testCase := range map[string]struct {
		pid, exe string
		stale    bool
	}{
		"current binary":  {"42", self, false},
		"replaced binary": {"43", self + " (deleted)", true},
		"other binary":    {"44", "/usr/local/bin/old-gproxy", true},
		"not running":     {"0", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(pidFile, []byte(testCase.pid+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if testCase.exe != "" {
				link := filepath.Join(procRoot, testCase.pid, "exe")
				if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
					t.Fatal(err)
				}
				_ = os.Remove(link)
				if err := os.Symlink(testCase.exe, link); err != nil {
					t.Fatal(err)
				}
			}
			stale, err := WatchdogRunsStaleBinary(context.Background())
			if err != nil || stale != testCase.stale {
				t.Fatalf("stale = %v, %v; want %v", stale, err, testCase.stale)
			}
		})
	}
}
