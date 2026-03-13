package core

import (
	"fmt"
	"runtime"
)

const (
	snellVersion     = "5.0.1"
	snellDownloadURL = "https://dl.nssurge.com/snell"
)

// SnellDownloadURL returns the download URL for the snell-server binary
// for the current architecture.
func SnellDownloadURL() string {
	arch := runtime.GOARCH
	switch arch {
	case "arm64":
		arch = "aarch64"
	}
	filename := fmt.Sprintf("snell-server-v%s-linux-%s.zip", snellVersion, arch)
	return snellDownloadURL + "/" + filename
}
