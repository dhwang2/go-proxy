package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically using a temp file + rename.
// It preserves original permissions, or uses 0600 for new files.
func AtomicWrite(path string, data []byte) error {
	return atomicWrite(path, data, 0, false)
}

// AtomicWriteMode is AtomicWrite with an explicit mode, applied whether or not
// the file already exists. Use it where the permissions are part of what the
// caller is deciding: a version receipt beside a 0755 binary has to be readable
// by whoever can run that binary, and a copy left at 0600 has to be corrected
// rather than preserved.
func AtomicWriteMode(path string, data []byte, perm os.FileMode) error {
	return atomicWrite(path, data, perm, true)
}

func atomicWrite(path string, data []byte, mode os.FileMode, force bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Preserve existing permissions unless the caller named a mode.
	perm := os.FileMode(0600)
	if force {
		perm = mode
	} else if info, err := os.Stat(path); err == nil {
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
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
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
	if err := os.WriteFile(bak, data, 0600); err != nil {
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
