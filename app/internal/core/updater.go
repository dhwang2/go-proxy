package core

import (
	"context"
	"fmt"
	"strings"

	"go-proxy/pkg/github"
	"go-proxy/pkg/sysutil"
)

// UpdateCheck checks if a newer version is available for a component.
type UpdateCheck struct {
	Component      Component
	CurrentVersion string
	LatestVersion  string
	UpdateAvail    bool
	DownloadURL    string
}

// HasRepo reports whether a component has a public GitHub repository for updates.
func HasRepo(c Component) bool {
	return componentRepo(c) != ""
}

// UpdatableComponents returns only components that can be checked for updates.
func UpdatableComponents() []Component {
	var out []Component
	for _, c := range AllComponents() {
		if HasRepo(c) {
			out = append(out, c)
		}
	}
	return out
}

// CheckUpdate checks GitHub for a newer release of a component.
func CheckUpdate(ctx context.Context, component Component, binPath string) (*UpdateCheck, error) {
	info := DetectVersion(binPath, component)
	repo := componentRepo(component)
	if repo == "" {
		return nil, fmt.Errorf("no repository configured for %s", component)
	}

	release, err := github.LatestRelease(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("check %s update: %w", component, err)
	}

	current := normalizeVersion(info.Version)
	latest := normalizeVersion(release.TagName)

	// Update is available if: not installed, or installed with different version.
	avail := !info.Installed || current != latest

	check := &UpdateCheck{
		Component:      component,
		CurrentVersion: info.Version,
		LatestVersion:  release.TagName,
		UpdateAvail:    avail,
		DownloadURL:    release.FindAssetURL(componentAssetPattern(component)),
	}
	return check, nil
}

// normalizeVersion strips the leading "v" prefix for comparison.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func componentRepo(c Component) string {
	switch c {
	case CompSingBox:
		return "SagerNet/sing-box"
	case CompSnell:
		return "" // private distribution
	case CompShadowTLS:
		return "ihciah/shadow-tls"
	case CompCaddy:
		return "caddyserver/caddy"
	default:
		return ""
	}
}

func componentAssetPattern(c Component) string {
	arch := sysutil.Arch()
	switch c {
	case CompSingBox:
		return fmt.Sprintf("sing-box-*-linux-%s", arch)
	case CompShadowTLS:
		return fmt.Sprintf("shadow-tls-*-linux-%s", arch)
	case CompCaddy:
		return fmt.Sprintf("caddy_*_linux_%s", arch)
	default:
		return ""
	}
}
