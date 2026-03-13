package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dhwang2/go-proxy/internal/update"
	gh "github.com/dhwang2/go-proxy/pkg/github"
)

// BinaryResult describes the outcome of an EnsureBinary call.
type BinaryResult struct {
	Name       string
	Path       string
	Downloaded bool
	Skipped    bool
	Err        error
}

// EnsureProtocolBinaries ensures the required binaries for a given protocol
// are present, downloading them if missing.
// Returns a list of results describing what happened for each binary.
func EnsureProtocolBinaries(ctx context.Context, workDir, proto string) []BinaryResult {
	if strings.TrimSpace(workDir) == "" {
		workDir = "/etc/go-proxy"
	}
	binDir := filepath.Join(workDir, "bin")

	switch proto {
	case "snell":
		r := ensureBinary(ctx, binDir, "snell-server", func() (string, string, error) {
			return SnellDownloadURL(), "snell-server", nil
		})
		return []BinaryResult{r}
	case "trojan", "vless", "tuic", "anytls", "ss":
		return ensureSingboxBinaries(ctx, binDir)
	default:
		return nil
	}
}

// ensureSingboxBinaries ensures sing-box is present (required for all
// sing-box-based protocols).
func ensureSingboxBinaries(ctx context.Context, binDir string) []BinaryResult {
	r := ensureBinary(ctx, binDir, "sing-box", func() (string, string, error) {
		return latestGitHubBinaryURL(ctx, "SagerNet/sing-box", "sing-box", runtime.GOARCH)
	})
	return []BinaryResult{r}
}

// ensureBinary checks if a binary exists and downloads it if missing.
// resolveURL returns (downloadURL, binaryNameInArchive, error).
func ensureBinary(ctx context.Context, binDir, binaryName string, resolveURL func() (string, string, error)) BinaryResult {
	binPath := filepath.Join(binDir, binaryName)
	result := BinaryResult{Name: binaryName, Path: binPath}

	if st, err := os.Stat(binPath); err == nil && !st.IsDir() {
		result.Skipped = true
		return result
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		result.Err = fmt.Errorf("mkdir %s: %w", binDir, err)
		return result
	}

	url, archiveBinary, err := resolveURL()
	if err != nil {
		result.Err = fmt.Errorf("resolve download url: %w", err)
		return result
	}

	tmp := filepath.Join(os.TempDir(), "binary-download-"+binaryName)
	defer os.Remove(tmp)

	if err := update.DownloadFile(ctx, url, "", tmp); err != nil {
		result.Err = fmt.Errorf("download %s: %w", binaryName, err)
		return result
	}

	lowerURL := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lowerURL, ".zip"):
		if err := update.ExtractZipBinary(tmp, archiveBinary, binPath); err != nil {
			result.Err = fmt.Errorf("extract %s from zip: %w", binaryName, err)
			return result
		}
	case strings.HasSuffix(lowerURL, ".tar.gz") || strings.HasSuffix(lowerURL, ".tgz"):
		if err := update.ExtractTarGzBinary(tmp, archiveBinary, binPath); err != nil {
			result.Err = fmt.Errorf("extract %s from tar.gz: %w", binaryName, err)
			return result
		}
	default:
		// Assume raw binary.
		data, err := os.ReadFile(tmp)
		if err != nil {
			result.Err = err
			return result
		}
		if err := os.WriteFile(binPath, data, 0o755); err != nil {
			result.Err = err
			return result
		}
	}

	result.Downloaded = true
	return result
}

// latestGitHubBinaryURL fetches the latest release from a GitHub repo
// and selects the appropriate asset for the current platform.
func latestGitHubBinaryURL(ctx context.Context, repo, binaryName, arch string) (string, string, error) {
	client := gh.NewClient()
	rel, err := client.LatestRelease(ctx, repo)
	if err != nil {
		return "", "", fmt.Errorf("fetch latest release for %s: %w", repo, err)
	}

	hints := []string{binaryName, "linux", arch}
	asset, err := update.SelectAssetForRuntime(rel.Assets, hints...)
	if err != nil {
		return "", "", fmt.Errorf("no matching asset for %s: %w", repo, err)
	}
	return asset.DownloadURL, binaryName, nil
}
