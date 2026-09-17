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
