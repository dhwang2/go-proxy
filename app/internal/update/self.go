package update

import (
	"context"
	"fmt"
	"os"

	"go-proxy/internal/core"
	"go-proxy/pkg/github"
)

const selfRepo = "dhwang2/go-proxy"

// SelfUpdateCheck checks if a newer version of go-proxy is available.
type SelfUpdateCheck struct {
	CurrentVersion string
	LatestVersion  string
	UpdateAvail    bool
	DownloadURL    string
}

// CheckSelfUpdate checks GitHub for a newer release of go-proxy.
func CheckSelfUpdate(ctx context.Context, currentVersion string) (*SelfUpdateCheck, error) {
	release, err := github.LatestRelease(ctx, selfRepo)
	if err != nil {
		return nil, fmt.Errorf("check self update: %w", err)
	}

	check := &SelfUpdateCheck{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		UpdateAvail:    currentVersion != release.TagName,
	}

	if check.UpdateAvail {
		check.DownloadURL = release.FindAssetURL("proxy-linux-")
	}
	return check, nil
}

// SelfUpdate downloads and replaces the current binary.
func SelfUpdate(ctx context.Context, downloadURL string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	return core.DownloadBinary(ctx, downloadURL, execPath)
}
