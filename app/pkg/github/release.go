package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Release struct {
	TagName string
	Name    string
	URL     string
	Assets  []ReleaseAsset
}

type ReleaseAsset struct {
	Name        string
	DownloadURL string
	Size        int64
	Checksum    string
}

type commitInfo struct {
	SHA string `json:"sha"`
}

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL: "https://api.github.com",
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) LatestRelease(ctx context.Context, repo string) (*Release, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil, fmt.Errorf("empty repo")
	}
	url := strings.TrimRight(c.baseURL(), "/") + "/repos/" + repo + "/releases/latest"
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
		return nil, fmt.Errorf("github latest release %s: http %d", repo, resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		URL     string `json:"html_url"`
		Assets  []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			APIURL string `json:"url"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := &Release{
		TagName: strings.TrimSpace(payload.TagName),
		Name:    strings.TrimSpace(payload.Name),
		URL:     strings.TrimSpace(payload.URL),
		Assets:  make([]ReleaseAsset, 0, len(payload.Assets)),
	}
	for _, a := range payload.Assets {
		downloadURL := strings.TrimSpace(a.APIURL)
		if downloadURL == "" {
			downloadURL = strings.TrimSpace(a.URL)
		}
		out.Assets = append(out.Assets, ReleaseAsset{
			Name:        strings.TrimSpace(a.Name),
			DownloadURL: downloadURL,
			Size:        a.Size,
		})
	}
	return out, nil
}

func (c *Client) BranchCommit(ctx context.Context, repo, branch string) (string, error) {
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if repo == "" || branch == "" {
		return "", fmt.Errorf("invalid repo/branch")
	}
	url := strings.TrimRight(c.baseURL(), "/") + "/repos/" + repo + "/commits/" + branch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("github branch commit %s@%s: http %d", repo, branch, resp.StatusCode)
	}
	var payload commitInfo
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.SHA), nil
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) baseURL() string {
	if c != nil && strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimSpace(c.BaseURL)
	}
	return "https://api.github.com"
}
