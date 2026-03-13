package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func BackupFile(path string, backupDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty source path")
	}
	if backupDir == "" {
		backupDir = filepath.Dir(path)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(backupDir, filepath.Base(path)+"."+time.Now().UTC().Format("20060102T150405")+".bak")
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}
