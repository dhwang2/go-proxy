package service

import (
	"path/filepath"
	"strings"
)

// ShadowTLSInstances lists shadow-tls systemd unit file names in the given directory.
func ShadowTLSInstances(systemdDir string) ([]string, error) {
	if strings.TrimSpace(systemdDir) == "" {
		systemdDir = "/etc/systemd/system"
	}
	matches, err := filepath.Glob(filepath.Join(systemdDir, "shadow-tls*.service"))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	return names, nil
}
