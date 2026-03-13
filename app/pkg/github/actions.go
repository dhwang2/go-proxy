package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type WorkflowRun struct {
	ID         int64
	HeadSHA    string
	HeadBranch string
	Status     string
	Conclusion string
}

type WorkflowArtifact struct {
	ID          int64
	Name        string
	DownloadURL string
	Expired     bool
	SizeInBytes int64
}

func (c *Client) LatestSuccessfulWorkflowRun(ctx context.Context, repo, workflowFile, branch string) (*WorkflowRun, error) {
	repo = strings.TrimSpace(repo)
	workflowFile = strings.TrimSpace(workflowFile)
	branch = strings.TrimSpace(branch)
	if repo == "" || workflowFile == "" || branch == "" {
		return nil, fmt.Errorf("invalid repo/workflow/branch")
	}
	url := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/runs?branch=%s&status=completed&per_page=20", strings.TrimRight(c.baseURL(), "/"), repo, workflowFile, branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github workflow runs %s %s@%s: http %d", repo, workflowFile, branch, resp.StatusCode)
	}
	var payload struct {
		WorkflowRuns []struct {
			ID         int64  `json:"id"`
			HeadSHA    string `json:"head_sha"`
			HeadBranch string `json:"head_branch"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	for _, run := range payload.WorkflowRuns {
		if strings.TrimSpace(run.Conclusion) != "success" {
			continue
		}
		return &WorkflowRun{
			ID:         run.ID,
			HeadSHA:    strings.TrimSpace(run.HeadSHA),
			HeadBranch: strings.TrimSpace(run.HeadBranch),
			Status:     strings.TrimSpace(run.Status),
			Conclusion: strings.TrimSpace(run.Conclusion),
		}, nil
	}
	return nil, fmt.Errorf("no successful workflow run found for %s %s@%s", repo, workflowFile, branch)
}

func (c *Client) ListWorkflowArtifacts(ctx context.Context, repo string, runID int64) ([]WorkflowArtifact, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" || runID <= 0 {
		return nil, fmt.Errorf("invalid repo/run id")
	}
	url := fmt.Sprintf("%s/repos/%s/actions/runs/%d/artifacts", strings.TrimRight(c.baseURL(), "/"), repo, runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github workflow artifacts %s run=%d: http %d", repo, runID, resp.StatusCode)
	}
	var payload struct {
		Artifacts []struct {
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			ArchiveDownloadURL string `json:"archive_download_url"`
			Expired            bool   `json:"expired"`
			SizeInBytes        int64  `json:"size_in_bytes"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]WorkflowArtifact, 0, len(payload.Artifacts))
	for _, item := range payload.Artifacts {
		out = append(out, WorkflowArtifact{
			ID:          item.ID,
			Name:        strings.TrimSpace(item.Name),
			DownloadURL: strings.TrimSpace(item.ArchiveDownloadURL),
			Expired:     item.Expired,
			SizeInBytes: item.SizeInBytes,
		})
	}
	return out, nil
}
