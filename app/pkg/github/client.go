package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Release holds information about a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

// Asset holds information about a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// FindAssetURL finds the first asset URL matching the given pattern.
// The pattern supports glob wildcards (e.g., "caddy_*_linux_amd64").
func (r *Release) FindAssetURL(pattern string) string {
	for _, a := range r.Assets {
		if matched, _ := filepath.Match(pattern, a.Name); matched {
			return a.BrowserDownloadURL
		}
		if strings.Contains(a.Name, pattern) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// LatestRelease fetches the latest release for a GitHub repository.
// repo format: "owner/repo"
func LatestRelease(ctx context.Context, repo string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api %s: status %d", repo, resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &release, nil
}

// Commit holds basic commit information.
type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// LatestCommit fetches the latest commit on a branch.
func LatestCommit(ctx context.Context, repo, branch string) (*Commit, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", repo, branch)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api: status %d", resp.StatusCode)
	}

	var result struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode commit: %w", err)
	}
	return &Commit{SHA: result.SHA, Message: result.Commit.Message}, nil
}
