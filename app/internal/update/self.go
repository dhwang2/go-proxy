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
	"go-proxy/pkg/fileutil"
	"go-proxy/pkg/github"
	"go-proxy/pkg/sysutil"
)

const selfRepo = "dhwang2/go-proxy"

type SelfUpdateCheck struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateAvail    bool   `json:"update_available"`
	// Updated is set once update has replaced the executable.
	Updated     bool   `json:"updated,omitempty"`
	DownloadURL string `json:"-"`
	Digest      string `json:"-"`
}

// ResolveSelfUpdate compares this build with the latest release. It never
// offers an older release: a newer or development build stays as it is.
func ResolveSelfUpdate(ctx context.Context, currentVersion string) (*SelfUpdateCheck, error) {
	release, err := github.LatestRelease(ctx, selfRepo)
	if err != nil {
		return nil, err
	}
	current := "v" + strings.TrimPrefix(currentVersion, "v")
	available := semver.IsValid(current) && semver.IsValid(release.TagName) && semver.Compare(release.TagName, current) > 0
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

// RefreshCompletions rewrites the installed shell completion scripts with the
// ones executable generates, so a fix to a script reaches a host that updates
// rather than reinstalls. A script that is absent stays absent: the CLI was
// built from source, or that shell's completion directory does not exist.
func RefreshCompletions(ctx context.Context, executable, bashPath, zshPath string) error {
	for _, item := range []struct{ shell, path string }{{"bash", bashPath}, {"zsh", zshPath}} {
		if _, err := os.Stat(item.path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		script, err := exec.CommandContext(ctx, executable, "completion", item.shell).Output()
		if err != nil || len(script) == 0 {
			return fmt.Errorf("%s completion could not be generated", item.shell)
		}
		if err := fileutil.AtomicWriteMode(item.path, script, 0o644); err != nil {
			return err
		}
	}
	return nil
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
