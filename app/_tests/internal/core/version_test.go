package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
)

func TestDetectSnellVersionFromStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snell-server")
	script := "#!/bin/sh\nprintf '%s\\n' '2026-09-13 23:52:43.440803 [server_main] <NOTIFY> snell-server v6.0.0 (Aug 7 2026)' >&2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	info := DetectVersion(context.Background(), path, CompSnell)
	if !info.Installed || info.Version != "v6.0.0" {
		t.Fatalf("unexpected Snell version: %+v", info)
	}
}

type snellDownloadRecorder struct{ urls []string }

func (r *snellDownloadRecorder) RoundTrip(request *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, request.URL.String())
	return nil, fmt.Errorf("fixture download blocked")
}

func TestEnsureSnellOnlyReusesKnownV6Runtime(t *testing.T) {
	for _, version := range []string{"v5.0.1", "v6.0.0beta3", "v6.0.0"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snell-server")
			script := "#!/bin/sh\nprintf '%s\\n' 'snell-server " + version + "' >&2\n"
			if err := os.WriteFile(path, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			// A stale receipt must not override the version reported by the executable.
			if err := os.WriteFile(path+".version", []byte(SnellVersion+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			originalPath, originalClient := config.SnellBin, http.DefaultClient
			recorder := &snellDownloadRecorder{}
			config.SnellBin = path
			http.DefaultClient = &http.Client{Transport: recorder}
			t.Cleanup(func() { config.SnellBin = originalPath; http.DefaultClient = originalClient })
			err := Ensure(context.Background(), CompSnell, "")
			if version == "v6.0.0" {
				if err != nil || len(recorder.urls) != 0 {
					t.Fatalf("known runtime was not reused: %v %v", err, recorder.urls)
				}
			} else {
				if err == nil || len(recorder.urls) != 1 {
					t.Fatalf("incompatible runtime bypassed download: %v %v", err, recorder.urls)
				}
				check, resolveErr := ResolveUpdate(context.Background(), CompSnell, path, "")
				if resolveErr != nil || !check.UpdateAvail || check.LatestVersion != SnellVersion || recorder.urls[0] != check.DownloadURL {
					t.Fatalf("wrong replacement release: %+v %v", check, resolveErr)
				}
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != script {
				t.Fatal("failed or skipped download changed the original binary")
			}
		})
	}
}

// `core version` and `core check` report one installation, so they have to read
// the same version of it. Snell RC2 identifies itself as v6.0.0 and the receipt
// beside the binary records which archive that was; reading the receipt in one
// command and not the other made the two commands contradict each other.
func TestInstalledVersionReadsTheSnellReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snell-server")
	script := "#!/bin/sh\nprintf '%s\\n' 'snell-server v6.0.0' >&2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if info := InstalledVersion(context.Background(), path, CompSnell); info.Version != "v6.0.0" {
		t.Fatalf("no receipt should leave the reported version alone: %+v", info)
	}
	if err := os.WriteFile(path+".version", []byte(SnellVersion+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := InstalledVersion(context.Background(), path, CompSnell)
	if !info.Installed || info.Version != SnellVersion {
		t.Fatalf("receipt was not read: %+v", info)
	}
	check, err := ResolveUpdate(context.Background(), CompSnell, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if check.CurrentVersion != info.Version {
		t.Fatalf("check reports %q where version reports %q", check.CurrentVersion, info.Version)
	}
	if !check.Installed || check.UpdateAvail {
		t.Fatalf("the verified archive is not an update over itself: %+v", check)
	}
}

// A receipt this user cannot read is the ordinary case: `core check` is a query
// that does not require root, and the receipt sits in a root-owned directory.
// The answer must still be "no update", because the archive that reports v6.0.0
// is the one this project installs.
func TestSnellNeedsNoUpdateWithoutItsReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snell-server")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' 'snell-server v6.0.0' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	check, err := ResolveUpdate(context.Background(), CompSnell, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !check.Installed || check.UpdateAvail {
		t.Fatalf("an unreadable receipt reported an update over itself: %+v", check)
	}
	// The version string stays what the executable said rather than claiming a
	// receipt that was never read.
	if check.CurrentVersion != "v6.0.0" {
		t.Fatalf("current version %q was not the executable self-report", check.CurrentVersion)
	}
}

// Nothing installed has nothing to update. Reporting an update available for an
// absent core sent a caller to `core update` for what is a first install, and
// `installed` is the field that actually says so.
func TestCheckReportsNoUpdateForAnAbsentCore(t *testing.T) {
	check, err := ResolveUpdate(context.Background(), CompSnell, filepath.Join(t.TempDir(), "absent"), "")
	if err != nil {
		t.Fatal(err)
	}
	if check.Installed || check.UpdateAvail {
		t.Fatalf("absent core reported as installed or updatable: %+v", check)
	}
	if !check.NeedsInstall() {
		t.Fatal("absent core reported as needing no install")
	}
}
