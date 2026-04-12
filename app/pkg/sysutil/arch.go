package sysutil

import "runtime"

// Arch returns the current platform architecture string for download URLs.
func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}
