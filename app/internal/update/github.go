package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	gh "github.com/dhwang2/go-proxy/pkg/github"
)

type CompareResult int

const (
	CompareUnknown CompareResult = iota
	CompareEqual
	CompareGreater
	CompareLess
)

const GoProxyCIWorkflowFile = "ci.yml"

func CompareTags(current, latest string) CompareResult {
	curr := parseTagVersion(current)
	next := parseTagVersion(latest)
	if len(curr) == 0 || len(next) == 0 {
		if normalizeTag(current) == normalizeTag(latest) {
			return CompareEqual
		}
		return CompareUnknown
	}
	max := len(curr)
	if len(next) > max {
		max = len(next)
	}
	for i := 0; i < max; i++ {
		c := 0
		n := 0
		if i < len(curr) {
			c = curr[i]
		}
		if i < len(next) {
			n = next[i]
		}
		if c == n {
			continue
		}
		if c > n {
			return CompareGreater
		}
		return CompareLess
	}
	return CompareEqual
}

func SelectAssetForRuntime(assets []gh.ReleaseAsset, extras ...string) (gh.ReleaseAsset, error) {
	if len(assets) == 0 {
		return gh.ReleaseAsset{}, errors.New("release has no assets")
	}

	// When hints are provided, find assets where ALL hints match as substrings.
	hints := make([]string, 0, len(extras))
	for _, e := range extras {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			hints = append(hints, e)
		}
	}
	if len(hints) > 0 {
		for _, asset := range assets {
			name := strings.ToLower(strings.TrimSpace(asset.Name))
			if matchAllHints(name, hints) {
				return asset, nil
			}
		}
		return gh.ReleaseAsset{}, fmt.Errorf("no asset matched hints %v", hints)
	}

	// Fallback: match by OS/arch candidates (used for go-proxy self-update).
	osName := strings.ToLower(runtime.GOOS)
	arch := strings.ToLower(runtime.GOARCH)
	candidates := []string{
		"proxy-" + osName + "-" + arch,
		"proxy_" + osName + "_" + arch,
		"proxy-" + arch,
	}
	if arch == "amd64" {
		candidates = append(candidates, "proxy")
	}
	if arch == "arm64" {
		candidates = append(candidates, "proxy-arm64")
	}
	for _, cand := range candidates {
		for _, asset := range assets {
			name := strings.ToLower(strings.TrimSpace(asset.Name))
			if name == cand {
				return asset, nil
			}
		}
	}
	for _, cand := range candidates {
		for _, asset := range assets {
			name := strings.ToLower(strings.TrimSpace(asset.Name))
			if strings.Contains(name, cand) {
				return asset, nil
			}
		}
	}
	return gh.ReleaseAsset{}, fmt.Errorf("no asset matched runtime %s/%s", osName, arch)
}

func matchAllHints(name string, hints []string) bool {
	for _, h := range hints {
		if !strings.Contains(name, h) {
			return false
		}
	}
	return true
}

func DownloadFile(ctx context.Context, url, token, dest string) error {
	return downloadFileWithAccept(ctx, url, token, dest, "application/octet-stream")
}

func DownloadArtifactArchive(ctx context.Context, url, token, dest string) error {
	return downloadFileWithAccept(ctx, url, token, dest, "")
}

func downloadFileWithAccept(ctx context.Context, url, token, dest, accept string) error {
	url = strings.TrimSpace(url)
	dest = strings.TrimSpace(dest)
	if url == "" || dest == "" {
		return fmt.Errorf("invalid download args")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(accept) != "" {
		req.Header.Set("Accept", strings.TrimSpace(accept))
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download failed: http %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}

func ExtractTarGzBinary(archivePath, binaryName, outPath string) error {
	archivePath = strings.TrimSpace(archivePath)
	binaryName = strings.TrimSpace(binaryName)
	outPath = strings.TrimSpace(outPath)
	if archivePath == "" || binaryName == "" || outPath == "" {
		return fmt.Errorf("invalid extract args")
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if hdr == nil || hdr.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimSpace(filepath.Base(hdr.Name))
		if name != binaryName {
			continue
		}
		tmp := outPath + ".tmp"
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		wf, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(wf, tr); err != nil {
			wf.Close()
			return err
		}
		if err := wf.Sync(); err != nil {
			wf.Close()
			return err
		}
		if err := wf.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp, outPath); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("binary %s not found in archive", binaryName)
}

func ExtractZipBinary(archivePath, binaryName, outPath string) error {
	archivePath = strings.TrimSpace(archivePath)
	binaryName = strings.TrimSpace(binaryName)
	outPath = strings.TrimSpace(outPath)
	if archivePath == "" || binaryName == "" || outPath == "" {
		return fmt.Errorf("invalid extract args")
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.TrimSpace(filepath.Base(file.Name)) != binaryName {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		tmp := outPath + ".tmp"
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		wf, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(wf, rc); err != nil {
			wf.Close()
			return err
		}
		if err := wf.Sync(); err != nil {
			wf.Close()
			return err
		}
		if err := wf.Close(); err != nil {
			return err
		}
		if err := os.Rename(tmp, outPath); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("binary %s not found in zip archive", binaryName)
}

func parseTagVersion(tag string) []int {
	tag = normalizeTag(tag)
	if tag == "" {
		return nil
	}
	parts := strings.Split(tag, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n := readLeadingNumber(p)
		if n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	re := regexp.MustCompile(`(?i)v?[0-9]+(?:\.[0-9]+){1,3}`)
	match := re.FindString(tag)
	if match == "" {
		return ""
	}
	match = strings.TrimPrefix(match, "v")
	match = strings.TrimPrefix(match, "V")
	return match
}

func readLeadingNumber(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return -1
	}
	buf := strings.Builder{}
	for _, r := range v {
		if r < '0' || r > '9' {
			break
		}
		buf.WriteRune(r)
	}
	if buf.Len() == 0 {
		return -1
	}
	n, err := strconv.Atoi(buf.String())
	if err != nil {
		return -1
	}
	return n
}
