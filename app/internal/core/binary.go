package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"go-proxy/pkg/fileutil"
)

// DownloadBinary downloads a binary from url and installs it to destPath.
func DownloadBinary(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := fileutil.AtomicWrite(destPath, data); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}

	// Make executable.
	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("chmod binary: %w", err)
	}

	return nil
}
