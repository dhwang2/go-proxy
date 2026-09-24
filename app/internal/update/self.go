package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"

	"go-proxy/internal/core"
	"go-proxy/pkg/github"
	"go-proxy/pkg/sysutil"
)

const selfRepo = "dhwang2/go-proxy"

type SelfUpdateCheck struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateAvail    bool   `json:"update_available"`
	DownloadURL    string `json:"-"`
	Digest         string `json:"-"`
}

func ResolveSelfUpdate(ctx context.Context, currentVersion, version string) (*SelfUpdateCheck, error) {
	var release *github.Release
	var err error
	if version == "" {
		release, err = github.LatestRelease(ctx, selfRepo)
	} else {
		release, err = github.ReleaseByTag(ctx, selfRepo, "v"+strings.TrimPrefix(version, "v"))
	}
	if err != nil {
		return nil, err
	}
	current := "v" + strings.TrimPrefix(currentVersion, "v")
	available := current != release.TagName
	if version == "" {
		available = semver.IsValid(current) && semver.IsValid(release.TagName) && semver.Compare(release.TagName, current) > 0
	}
	check := &SelfUpdateCheck{CurrentVersion: currentVersion, LatestVersion: release.TagName, UpdateAvail: available}
	assetName := "gproxy-linux-" + sysutil.Arch()
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			check.DownloadURL = asset.BrowserDownloadURL
			check.Digest = asset.Digest
			break
		}
	}
	if check.DownloadURL == "" {
		return nil, fmt.Errorf("release has no binary for this architecture")
	}
	if !strings.HasPrefix(check.Digest, "sha256:") {
		return nil, fmt.Errorf("release has no sha256 digest")
	}
	return check, nil
}

func SelfUpdate(ctx context.Context, check *SelfUpdateCheck) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return err
	}
	return core.InstallBinary(ctx, check.DownloadURL, check.Digest, execPath, "gproxy", func(staged string) error {
		out, err := exec.CommandContext(ctx, staged, "version").Output()
		if err != nil {
			return fmt.Errorf("updated executable failed version validation")
		}
		for _, field := range strings.Fields(string(out)) {
			if strings.Trim(field, "\",:") == check.LatestVersion {
				return nil
			}
		}
		return fmt.Errorf("updated executable version mismatch")
	})
}
