package update

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	gh "github.com/dhwang2/go-proxy/pkg/github"
)

func TestReadLocalRef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ref")
	if err := os.WriteFile(p, []byte("branch=main sha=abcdef1234567890\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readLocalRef(p); got != "abcdef1234567890" {
		t.Fatalf("unexpected ref: %s", got)
	}
}

func TestRunRepoModeFromArtifactZip(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "proxy")
	if err := os.WriteFile(target, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	artifactName := "go-proxy-linux-" + runtime.GOARCH
	binaryName := "proxy"
	if runtime.GOARCH == "arm64" {
		binaryName = "proxy-arm64"
	}
	archive := buildZipBinary(t, binaryName, []byte("new-binary"))

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/dhwang2/go-proxy/actions/workflows/ci.yml/runs":
			payload := map[string]any{
				"workflow_runs": []map[string]any{
					{
						"id":          101,
						"head_sha":    "abcdef1234567890",
						"head_branch": "main",
						"status":      "completed",
						"conclusion":  "success",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case "/repos/dhwang2/go-proxy/actions/runs/101/artifacts":
			payload := map[string]any{
				"artifacts": []map[string]any{
					{
						"id":                   201,
						"name":                 artifactName,
						"archive_download_url": srv.URL + "/artifacts/201/zip",
						"expired":              false,
						"size_in_bytes":        int64(len(archive)),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		case "/artifacts/201/zip":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := gh.NewClient()
	client.BaseURL = srv.URL
	client.HTTP = srv.Client()

	result, err := runRepoMode(context.Background(), client, Options{
		Repo:       "dhwang2/go-proxy",
		Branch:     "main",
		WorkDir:    workDir,
		BinaryPath: target,
	})
	if err != nil {
		t.Fatalf("runRepoMode error: %v", err)
	}
	if !result.Updated {
		t.Fatalf("expected repo update to be applied")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new-binary" {
		t.Fatalf("unexpected target content: %s", string(content))
	}
	refContent, err := os.ReadFile(filepath.Join(workDir, ".script_source_ref"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(refContent)) != "abcdef1234567890" {
		t.Fatalf("unexpected source ref: %s", strings.TrimSpace(string(refContent)))
	}
}

func buildZipBinary(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	})
	if err != nil {
		t.Fatalf("create zip header: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("write zip content: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
