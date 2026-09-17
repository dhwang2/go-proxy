package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSelfUpdateHelperProcess(t *testing.T) {
	if os.Getenv("GPROXY_UPDATE_HELPER") != "1" {
		return
	}
	executable, err := os.Executable()
	if err != nil || executable != os.Getenv("GPROXY_UPDATE_COPY") {
		t.Fatal("helper must update only its explicitly selected copy")
	}
	check := SelfUpdateCheck{LatestVersion: "v9.9.9-fixture", DownloadURL: os.Getenv("GPROXY_UPDATE_URL"), Digest: os.Getenv("GPROXY_UPDATE_DIGEST")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := SelfUpdate(ctx, &check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestSelfUpdateReplacesOnlyHelperCopyAfterValidation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	originalHash := sha256.Sum256(original)
	for _, scenario := range []string{"success", "bad-checksum", "wrong-version"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			copyPath := filepath.Join(dir, "gproxy-helper")
			source, err := os.Open(executable)
			if err != nil {
				t.Fatal(err)
			}
			dest, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
			if err != nil {
				source.Close()
				t.Fatal(err)
			}
			_, copyErr := io.Copy(dest, source)
			source.Close()
			closeErr := dest.Close()
			if copyErr != nil || closeErr != nil {
				t.Fatalf("copy helper: %v %v", copyErr, closeErr)
			}
			version := "v9.9.9-fixture"
			if scenario == "wrong-version" {
				version = "v9.9.8-fixture"
			}
			payload := []byte("#!/bin/sh\nif [ \"$1\" = version ]; then printf '%s\\n' 'go-proxy " + version + "'; exit 0; fi\nexit 2\n")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(payload) }))
			defer server.Close()
			checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))
			if scenario == "bad-checksum" {
				checksum = "sha256:" + strings.Repeat("0", 64)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, copyPath, "-test.run=^TestSelfUpdateHelperProcess$")
			command.Env = append(os.Environ(), "GPROXY_UPDATE_HELPER=1", "GPROXY_UPDATE_COPY="+copyPath, "GPROXY_UPDATE_URL="+server.URL+"/gproxy", "GPROXY_UPDATE_DIGEST="+checksum)
			output, runErr := command.CombinedOutput()
			if (runErr == nil) != (scenario == "success") {
				t.Fatalf("unexpected update result: %v %s", runErr, output)
			}
			after, err := os.ReadFile(copyPath)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "success" {
				if !bytes.Equal(after, payload) {
					t.Fatal("successful self-update did not replace the helper copy")
				}
				output, err := exec.CommandContext(ctx, copyPath, "version").Output()
				if err != nil || strings.TrimSpace(string(output)) != "go-proxy v9.9.9-fixture" {
					t.Fatalf("replacement cannot execute: %q %v", output, err)
				}
			} else if sha256.Sum256(after) != originalHash {
				t.Fatal("rejected update changed the helper executable")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "gproxy-helper" {
				t.Fatalf("update leaked staging files: %v", entries)
			}
		})
	}
	unchanged, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(unchanged) != originalHash {
		t.Fatal("test changed the original test binary")
	}
}
