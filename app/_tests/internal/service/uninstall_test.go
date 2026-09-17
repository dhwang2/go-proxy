package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUninstallOwnedFilesystemAndActiveLockPreservation(t *testing.T) {
	for _, failStop := range []bool{false, true} {
		name := "success"
		if failStop {
			name = "stop-failure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtimeDir := filepath.Join(root, "runtime")
			coreDir := filepath.Join(runtimeDir, "bin")
			unitDir := filepath.Join(root, "systemd")
			lockDir := filepath.Join(root, "run", "lock", "go-proxy")
			for _, dir := range []string{coreDir, unitDir, lockDir, filepath.Join(root, "tmpfiles.d")} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			watchdog := filepath.Join(unitDir, "proxy-watchdog.service")
			singBox := filepath.Join(unitDir, "sing-box.service")
			foreignUnit := filepath.Join(unitDir, "foreign.service")
			foreignData := filepath.Join(root, "foreign-data")
			setup := filepath.Join(root, "tmpfiles.d", "go-proxy.conf")
			configFile := filepath.Join(runtimeDir, "configuration.json")
			files := map[string]string{
				watchdog:                           "[Service]\nExecStart=" + filepath.Join(root, "gproxy") + " watchdog\n",
				singBox:                            "[Service]\nExecStart=" + filepath.Join(coreDir, "sing-box") + " run\n",
				foreignUnit:                        "[Service]\nExecStart=/usr/bin/unrelated\n",
				foreignData:                        "unrelated user data",
				configFile:                         "owned configuration",
				filepath.Join(coreDir, "sing-box"): "owned binary",
				setup:                              "owned tmpfiles rule",
			}
			for path, content := range files {
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(foreignData, filepath.Join(runtimeDir, "foreign-link")); err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(lockDir, ".operation.lock")
			lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
				t.Fatal(err)
			}
			before, err := lock.Stat()
			if err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(lockDir, ".state.lock")
			if err := os.WriteFile(statePath, []byte{}, 0600); err != nil {
				t.Fatal(err)
			}
			commandDir := filepath.Join(root, "commands")
			if err := os.Mkdir(commandDir, 0755); err != nil {
				t.Fatal(err)
			}
			calls := filepath.Join(root, "systemctl.calls")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GPROXY_TEST_UNINSTALL_CALLS\"\nif [ \"$GPROXY_TEST_UNINSTALL_FAIL\" = 1 ] && [ \"$1\" = stop ] && [ \"$2\" = sing-box ]; then exit 1; fi\nexit 0\n"
			if err := os.WriteFile(filepath.Join(commandDir, "systemctl"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", commandDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GPROXY_TEST_UNINSTALL_CALLS", calls)
			fail := "0"
			if failStop {
				fail = "1"
			}
			t.Setenv("GPROXY_TEST_UNINSTALL_FAIL", fail)
			paths, err := ownedUnitPaths([]string{watchdog, singBox}, coreDir, watchdog)
			if err != nil {
				t.Fatal(err)
			}
			stateUsed := false
			err = uninstallOwned(context.Background(), paths, runtimeDir, setup, func(fn func() error) error { stateUsed = true; return fn() })
			if (err != nil) != failStop {
				t.Fatalf("unexpected cleanup result: %v", err)
			}
			log, readErr := os.ReadFile(calls)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !strings.HasPrefix(string(log), "stop proxy-watchdog\ndisable proxy-watchdog\n") {
				t.Fatalf("watchdog was not stopped first: %s", log)
			}
			if failStop {
				if stateUsed {
					t.Fatal("cleanup proceeded after service stop failure")
				}
				for _, path := range []string{watchdog, singBox, configFile, setup} {
					if _, err := os.Stat(path); err != nil {
						t.Fatalf("failed stop removed %s", path)
					}
				}
			} else {
				if !stateUsed {
					t.Fatal("filesystem cleanup skipped the state boundary")
				}
				for _, path := range []string{watchdog, singBox, runtimeDir, setup} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("owned resource remains: %s %v", path, err)
					}
				}
				if !strings.HasSuffix(string(log), "daemon-reload\n") {
					t.Fatalf("removed units were not reloaded: %s", log)
				}
			}
			for _, path := range []string{foreignUnit, foreignData} {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != files[path] {
					t.Fatalf("unrelated resource changed: %s %v", path, err)
				}
			}
			after, err := os.Stat(lockPath)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("cleanup unlinked the active operation lock")
			}
			if _, err := os.Stat(statePath); err != nil {
				t.Fatal("cleanup removed the runtime state lock")
			}
			competing, err := os.OpenFile(lockPath, os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer competing.Close()
			if err := unix.Flock(int(competing.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
				t.Fatal("cleanup allowed a competing process to bypass the existing lock")
			}
		})
	}
}

func TestOwnedUnitInventoryRejectsForeignResourceWithoutChanges(t *testing.T) {
	root := t.TempDir()
	foreign := filepath.Join(root, "sing-box.service")
	content := []byte("[Service]\nExecStart=/usr/bin/unrelated\n")
	if err := os.WriteFile(foreign, content, 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := ownedUnitPaths([]string{foreign}, filepath.Join(root, "owned-bin"), filepath.Join(root, "proxy-watchdog.service"))
	if err == nil || len(paths) != 0 {
		t.Fatal("foreign service accepted for deletion")
	}
	after, readErr := os.ReadFile(foreign)
	if readErr != nil || string(after) != string(content) {
		t.Fatal("ownership validation modified the foreign resource")
	}
}
