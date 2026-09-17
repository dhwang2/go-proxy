package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// InstallBinary stages the archive and executable on disk before atomic replacement.
func InstallBinary(ctx context.Context, source, digest, destPath, binaryName string, validate func(string) error) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destPath), ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	archivePath := filepath.Join(stage, "archive")
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		f.Close()
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.Close()
		return fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		f.Close()
		return fmt.Errorf("download binary: status %d", resp.StatusCode)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hash), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("download binary: %w", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	if digest == "" || !strings.EqualFold(strings.TrimPrefix(digest, "sha256:"), actual) {
		return fmt.Errorf("binary checksum mismatch or unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stagedBinary := filepath.Join(stage, "binary")
	parsed, err := url.Parse(source)
	if err != nil {
		return err
	}
	if err := extractBinary(ctx, archivePath, stagedBinary, parsed.Path, binaryName); err != nil {
		return err
	}
	if err := os.Chmod(stagedBinary, 0755); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(stagedBinary); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(stagedBinary, destPath)
}

func extractBinary(ctx context.Context, source, dest, archiveName, binaryName string) error {
	write := func(r io.Reader) error {
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, contextReader{ctx, r})
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	if strings.HasSuffix(archiveName, ".zip") {
		z, err := zip.OpenReader(source)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			if filepath.Base(f.Name) != binaryName || !f.Mode().IsRegular() {
				continue
			}
			r, err := f.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			return write(r)
		}
		return fmt.Errorf("archive does not contain %s", binaryName)
	}
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	if strings.HasSuffix(archiveName, ".tar.gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		tr := tar.NewReader(contextReader{ctx, gz})
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if filepath.Base(header.Name) == binaryName && header.Typeflag == tar.TypeReg {
				return write(tr)
			}
		}
		return fmt.Errorf("archive does not contain %s", binaryName)
	}
	return write(f)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
