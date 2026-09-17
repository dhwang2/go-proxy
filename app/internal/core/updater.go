package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/pkg/fileutil"
	"go-proxy/pkg/github"
	"go-proxy/pkg/sysutil"
)

const SnellVersion = "6.0.0rc2"

type UpdateCheck struct {
	Component      Component `json:"component"`
	CurrentVersion string    `json:"current_version"`
	CurrentError   string    `json:"current_error,omitempty"`
	LatestVersion  string    `json:"latest_version"`
	UpdateAvail    bool      `json:"update_available"`
	DownloadURL    string    `json:"-"`
	Digest         string    `json:"-"`
}

func BinaryPath(c Component) string {
	switch c {
	case CompSingBox:
		return config.SingBoxBin
	case CompSnell:
		return config.SnellBin
	case CompShadowTLS:
		return config.ShadowTLSBin
	case CompCaddy:
		return config.CaddyBin
	}
	return ""
}

func CheckUpdate(ctx context.Context, component Component, binPath string) (*UpdateCheck, error) {
	return ResolveUpdate(ctx, component, binPath, "")
}

func ResolveUpdate(ctx context.Context, component Component, binPath, version string) (*UpdateCheck, error) {
	info := DetectVersion(ctx, binPath, component)
	check := &UpdateCheck{Component: component, CurrentVersion: info.Version, CurrentError: info.Error}
	if component == CompSnell {
		if version != "" && normalizeVersion(version) != SnellVersion {
			return nil, fmt.Errorf("supported snell version is %s", SnellVersion)
		}
		arch := sysutil.Arch()
		switch arch {
		case "amd64":
			check.Digest = "8a9c4463ca87cfa5eaa37c6af0d37ab93ea275aa12391985bb2a375ca3abd7f2"
		case "arm64":
			arch = "aarch64"
			check.Digest = "a0b2915cbc77dc3baf8fa069e741c20808d8a10c3a8a93e709a0a580645c3bd7"
		default:
			return nil, fmt.Errorf("unsupported snell architecture")
		}
		check.LatestVersion = SnellVersion
		check.DownloadURL = fmt.Sprintf("https://dl.nssurge.com/snell/snell-server-v%s-linux-%s.zip", SnellVersion, arch)
		// RC2 identifies itself as v6.0.0; remember the verified archive version.
		if normalizeVersion(info.Version) == "6.0.0" {
			if data, err := os.ReadFile(binPath + ".version"); err == nil {
				check.CurrentVersion = strings.TrimSpace(string(data))
			}
		}
	} else {
		repo := componentRepo(component)
		if repo == "" {
			return nil, fmt.Errorf("unknown core component")
		}
		var release *github.Release
		var err error
		if version == "" {
			release, err = github.LatestRelease(ctx, repo)
		} else {
			release, err = github.ReleaseByTag(ctx, repo, "v"+normalizeVersion(version))
		}
		if err != nil {
			return nil, err
		}
		check.LatestVersion = release.TagName
		for _, asset := range release.Assets {
			matched, _ := filepath.Match(componentAssetPattern(component), asset.Name)
			if matched {
				check.DownloadURL = asset.BrowserDownloadURL
				check.Digest = asset.Digest
				break
			}
		}
		if check.DownloadURL == "" {
			return nil, fmt.Errorf("release has no binary for %s", component)
		}
		if check.Digest == "" && component == CompShadowTLS && release.TagName == "v0.2.25" {
			switch sysutil.Arch() {
			case "amd64":
				check.Digest = "sha256:a173f5f2d57f45211b68e10ceeddc15b1791077b914fa89747bc705fddc71532"
			case "arm64":
				check.Digest = "sha256:3295476b37f549a68906519d3eaecb74bf3b6eaf9094cebb16ee84f0151373c6"
			}
		}
		if !strings.HasPrefix(check.Digest, "sha256:") {
			return nil, fmt.Errorf("release has no sha256 digest for %s", component)
		}
	}
	check.UpdateAvail = !info.Installed || normalizeVersion(check.CurrentVersion) != normalizeVersion(check.LatestVersion)
	return check, nil
}

func Ensure(ctx context.Context, component Component, version string) error {
	path := BinaryPath(component)
	if path == "" {
		return fmt.Errorf("unknown core component")
	}
	if version == "" {
		if info := DetectVersion(ctx, path, component); info.Installed && info.Error == "" && (component != CompSnell || normalizeVersion(info.Version) == "6.0.0") {
			return nil
		}
	}
	check, err := ResolveUpdate(ctx, component, path, version)
	if err != nil {
		return err
	}
	if !check.UpdateAvail {
		return nil
	}
	return ApplyUpdate(ctx, check)
}

func ApplyUpdate(ctx context.Context, check *UpdateCheck) error {
	path := BinaryPath(check.Component)
	err := InstallBinary(ctx, check.DownloadURL, check.Digest, path, filepath.Base(path), func(staged string) error {
		info := DetectVersion(ctx, staged, check.Component)
		if !info.Installed || info.Error != "" || info.Version == "unknown" {
			return fmt.Errorf("downloaded %s binary failed version validation", check.Component)
		}
		expected := normalizeVersion(check.LatestVersion)
		if check.Component == CompSnell {
			expected = "6.0.0"
		}
		if normalizeVersion(info.Version) != expected {
			return fmt.Errorf("downloaded %s version mismatch", check.Component)
		}
		if check.Component == CompSingBox {
			if _, err := os.Stat(config.SingBoxConfig); err == nil {
				if err := exec.CommandContext(ctx, staged, "check", "-c", config.SingBoxConfig).Run(); err != nil {
					return fmt.Errorf("downloaded sing-box rejected current configuration")
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if check.Component == CompSnell {
		return fileutil.AtomicWrite(path+".version", []byte(check.LatestVersion+"\n"))
	}
	return nil
}

func normalizeVersion(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }
func HasRepo(c Component) bool         { return componentRepo(c) != "" }
func UpdatableComponents() []Component { return AllComponents() }
func componentRepo(c Component) string {
	switch c {
	case CompSingBox:
		return "SagerNet/sing-box"
	case CompShadowTLS:
		return "ihciah/shadow-tls"
	case CompCaddy:
		return "caddyserver/caddy"
	}
	return ""
}
func componentAssetPattern(c Component) string {
	arch := sysutil.Arch()
	switch c {
	case CompSingBox:
		return fmt.Sprintf("sing-box-*-linux-%s.tar.gz", arch)
	case CompShadowTLS:
		if arch == "arm64" {
			arch = "aarch64"
		} else {
			arch = "x86_64"
		}
		return "shadow-tls-" + arch + "-unknown-linux-musl"
	case CompCaddy:
		return fmt.Sprintf("caddy_*_linux_%s.tar.gz", arch)
	}
	return ""
}
