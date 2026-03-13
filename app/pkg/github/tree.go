package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// TreeDiff represents a changed file between local and remote trees.
type TreeDiff struct {
	Path string
	SHA  string
}

// TreeEntry represents a single entry in a GitHub git tree.
type TreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob" or "tree"
	SHA  string `json:"sha"`
	Size int64  `json:"size"`
}

// FetchTree retrieves the git tree for a given SHA/branch from GitHub API.
func (c *Client) FetchTree(ctx context.Context, repo, sha string) ([]TreeEntry, error) {
	repo = strings.TrimSpace(repo)
	sha = strings.TrimSpace(sha)
	if repo == "" || sha == "" {
		return nil, fmt.Errorf("invalid repo or sha")
	}
	url := strings.TrimRight(c.baseURL(), "/") + "/repos/" + repo + "/git/trees/" + sha + "?recursive=1"
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
		return nil, fmt.Errorf("github tree %s@%s: http %d", repo, sha, resp.StatusCode)
	}
	var payload struct {
		Tree      []TreeEntry `json:"tree"`
		Truncated bool        `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Tree, nil
}

// DiffTrees compares local file SHAs with remote tree to find changed files.
func DiffTrees(local map[string]string, remote []TreeEntry) []TreeDiff {
	diffs := make([]TreeDiff, 0)

	// Build remote lookup for blobs only.
	remoteMap := make(map[string]string, len(remote))
	for _, entry := range remote {
		if entry.Type == "blob" {
			remoteMap[entry.Path] = entry.SHA
		}
	}

	// Find files changed or added in remote.
	for _, entry := range remote {
		if entry.Type != "blob" {
			continue
		}
		localSHA, exists := local[entry.Path]
		if !exists || localSHA != entry.SHA {
			diffs = append(diffs, TreeDiff{
				Path: entry.Path,
				SHA:  entry.SHA,
			})
		}
	}

	// Find files deleted in remote (present locally but not remotely).
	for path := range local {
		if _, exists := remoteMap[path]; !exists {
			diffs = append(diffs, TreeDiff{
				Path: path,
				SHA:  "",
			})
		}
	}

	return diffs
}
