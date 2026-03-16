package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically using a temp file + rename.
// It preserves the original file's permissions, or uses 0644 for new files.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Preserve existing permissions.
	perm := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// Backup creates a .bak copy of path. Returns the backup path.
// Returns empty string and nil error if the source file does not exist.
func Backup(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	bak := path + ".bak"
	if err := os.WriteFile(bak, data, 0644); err != nil {
		return "", fmt.Errorf("write backup %s: %w", bak, err)
	}
	return bak, nil
}

// RestoreBackup restores a .bak file to the original path.
func RestoreBackup(path string) error {
	bak := path + ".bak"
	if _, err := os.Stat(bak); os.IsNotExist(err) {
		return nil
	}
	return os.Rename(bak, path)
}

// CleanBackup removes the .bak file for path.
func CleanBackup(path string) {
	os.Remove(path + ".bak")
}
