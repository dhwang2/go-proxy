package core

import (
	"context"
	"fmt"

	"go-proxy/pkg/github"
)

// UpdateCheck checks if a newer version is available for a component.
type UpdateCheck struct {
	Component      Component
	CurrentVersion string
	LatestVersion  string
	UpdateAvail    bool
	DownloadURL    string
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

	check := &UpdateCheck{
		Component:      component,
		CurrentVersion: info.Version,
		LatestVersion:  release.TagName,
		UpdateAvail:    info.Version != release.TagName,
	}

	if check.UpdateAvail {
		check.DownloadURL = release.FindAssetURL(componentAssetPattern(component))
	}
	return check, nil
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
	switch c {
	case CompSingBox:
		return "sing-box-*-linux-"
	case CompShadowTLS:
		return "shadow-tls-*-linux-"
	case CompCaddy:
		return "caddy_*_linux_"
	default:
		return ""
	}
}
