package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestSuccessfulWorkflowRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/dhwang2/go-proxy/actions/workflows/ci.yml/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		payload := map[string]any{
			"workflow_runs": []map[string]any{
				{
					"id":          101,
					"head_sha":    "abc123",
					"head_branch": "main",
					"status":      "completed",
					"conclusion":  "success",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	client := NewClient()
	client.BaseURL = srv.URL
	client.HTTP = srv.Client()

	run, err := client.LatestSuccessfulWorkflowRun(context.Background(), "dhwang2/go-proxy", "ci.yml", "main")
	if err != nil {
		t.Fatalf("LatestSuccessfulWorkflowRun error: %v", err)
	}
	if run.ID != 101 || run.HeadSHA != "abc123" {
		t.Fatalf("unexpected run: %+v", run)
	}
}

func TestListWorkflowArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/dhwang2/go-proxy/actions/runs/101/artifacts" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		payload := map[string]any{
			"artifacts": []map[string]any{
				{
					"id":                   201,
					"name":                 "go-proxy-repo-linux-amd64",
					"archive_download_url": "https://example.com/artifacts/201/zip",
					"expired":              false,
					"size_in_bytes":        123,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	client := NewClient()
	client.BaseURL = srv.URL
	client.HTTP = srv.Client()

	artifacts, err := client.ListWorkflowArtifacts(context.Background(), "dhwang2/go-proxy", 101)
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("unexpected artifacts count: %d", len(artifacts))
	}
	if artifacts[0].Name != "go-proxy-repo-linux-amd64" {
		t.Fatalf("unexpected artifact: %+v", artifacts[0])
	}
}
