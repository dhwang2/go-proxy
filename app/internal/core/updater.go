package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/dhwang2/go-proxy/internal/update"
	"github.com/dhwang2/go-proxy/pkg/fileutil"
	gh "github.com/dhwang2/go-proxy/pkg/github"
)

type ComponentUpdate struct {
	Name        string
	Repo        string
	Installed   bool
	Current     string
	Latest      string
	NeedsUpdate bool
	Applied     bool
	DownloadURL string
	AssetName   string
	Message     string
	Err         error
}

type UpdateOptions struct {
	WorkDir    string
	Component  string
	Token      string
	DryRun     bool
	OnlyCheck  bool
	AllowMajor bool
}

func Update() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rows, err := ApplyUpdates(ctx, UpdateOptions{WorkDir: "/etc/go-proxy", Component: "all"})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Err != nil {
			return fmt.Errorf("%s update failed: %w", row.Name, row.Err)
		}
	}
	return nil
}

func CheckUpdates(ctx context.Context, opts UpdateOptions) ([]ComponentUpdate, error) {
	metas := defaultCoreComponents(opts.WorkDir)
	versions, err := CheckVersion(ctx, VersionOptions{WorkDir: opts.WorkDir})
	if err != nil {
		return nil, err
	}
	versionMap := map[string]ComponentVersion{}
	for _, v := range versions {
		versionMap[v.Name] = v
	}

	client := gh.NewClient()
	client.Token = strings.TrimSpace(opts.Token)
	updates := make([]ComponentUpdate, 0, len(metas))
	for _, meta := range metas {
		row := ComponentUpdate{Name: meta.Name, Repo: meta.Repo}
		if v, ok := versionMap[meta.Name]; ok {
			row.Installed = v.Installed
			row.Current = v.Version
			if v.Err != nil {
				row.Message = v.Err.Error()
			}
		}
		// snell-v5 uses a direct download URL instead of GitHub releases.
		if meta.Name == "snell-v5" {
			row.Latest = "v" + snellVersion
			row.DownloadURL = SnellDownloadURL()
			row.AssetName = filepath.Base(row.DownloadURL)
			if !row.Installed {
				row.NeedsUpdate = true
			} else {
				cmp := update.CompareTags(row.Current, row.Latest)
				row.NeedsUpdate = cmp == update.CompareLess
			}
			updates = append(updates, row)
			continue
		}
		if strings.TrimSpace(meta.Repo) == "" {
			row.Message = "no release repo configured"
			updates = append(updates, row)
			continue
		}
		rel, err := client.LatestRelease(ctx, meta.Repo)
		if err != nil {
			row.Err = err
			updates = append(updates, row)
			continue
		}
		row.Latest = strings.TrimSpace(rel.TagName)
		if !row.Installed && strings.TrimSpace(row.Latest) != "" {
			row.NeedsUpdate = true
		} else {
			cmp := update.CompareTags(row.Current, row.Latest)
			row.NeedsUpdate = cmp == update.CompareLess
		}
		if asset, err := selectCoreAsset(meta, rel.Assets); err == nil {
			row.DownloadURL = asset.DownloadURL
			row.AssetName = asset.Name
		}
		updates = append(updates, row)
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].Name < updates[j].Name })
	return updates, nil
}

func ApplyUpdates(ctx context.Context, opts UpdateOptions) ([]ComponentUpdate, error) {
	updates, err := CheckUpdates(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.OnlyCheck {
		return updates, nil
	}
	component := strings.ToLower(strings.TrimSpace(opts.Component))
	metas := defaultCoreComponents(opts.WorkDir)
	metaByName := map[string]coreComponent{}
	for _, m := range metas {
		metaByName[m.Name] = m
	}

	for i := range updates {
		u := &updates[i]
		if component != "" && component != "all" && component != strings.ToLower(u.Name) {
			continue
		}
		if !u.NeedsUpdate || u.DownloadURL == "" || u.Err != nil {
			continue
		}
		meta := metaByName[u.Name]
		if meta.BinaryPath == "" {
			continue
		}
		if dir := filepath.Dir(meta.BinaryPath); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				u.Err = fmt.Errorf("mkdir %s: %w", dir, err)
				continue
			}
		}
		tmp := filepath.Join(os.TempDir(), "core-update-"+strings.ReplaceAll(u.Name, " ", "-"))
		if err := update.DownloadFile(ctx, u.DownloadURL, opts.Token, tmp); err != nil {
			u.Err = err
			continue
		}
		defer os.Remove(tmp)
		if opts.DryRun {
			u.Message = "dry-run: asset downloaded"
			continue
		}
		incoming := tmp
		assetName := u.AssetName
		if assetName == "" {
			assetName = filepath.Base(u.DownloadURL)
		}
		if isZipArchive(assetName) {
			incoming = tmp + ".bin"
			if err := update.ExtractZipBinary(tmp, meta.BinaryName, incoming); err != nil {
				u.Err = err
				continue
			}
			defer os.Remove(incoming)
		} else if isTarArchive(assetName) || isTarArchive(filepath.Base(tmp)) {
			incoming = tmp + ".bin"
			if err := update.ExtractTarGzBinary(tmp, meta.BinaryName, incoming); err != nil {
				u.Err = err
				continue
			}
			defer os.Remove(incoming)
		}
		backup, err := replaceCoreBinary(meta.BinaryPath, incoming)
		if err != nil {
			u.Err = err
			continue
		}
		u.Applied = true
		u.Message = "updated"
		if backup != "" {
			u.Message += " (backup: " + backup + ")"
		}
		u.Current = u.Latest
		u.NeedsUpdate = false
	}
	return updates, nil
}

type coreComponent struct {
	Name       string
	Repo       string
	BinaryPath string
	BinaryName string
	Hints      []string
}

func defaultCoreComponents(workDir string) []coreComponent {
	if strings.TrimSpace(workDir) == "" {
		workDir = "/etc/go-proxy"
	}
	binDir := filepath.Join(workDir, "bin")
	arch := runtime.GOARCH
	// shadow-tls uses Rust target triples (x86_64/aarch64) instead of Go arch names.
	rustArch := arch
	switch arch {
	case "amd64":
		rustArch = "x86_64"
	case "arm64":
		rustArch = "aarch64"
	}
	return []coreComponent{
		{Name: "sing-box", Repo: "SagerNet/sing-box", BinaryPath: filepath.Join(binDir, "sing-box"), BinaryName: "sing-box", Hints: []string{"sing-box", "linux", arch}},
		{Name: "shadow-tls", Repo: "ihciah/shadow-tls", BinaryPath: filepath.Join(binDir, "shadow-tls"), BinaryName: "shadow-tls", Hints: []string{"shadow-tls", "linux", rustArch}},
		{Name: "caddy", Repo: "caddyserver/caddy", BinaryPath: filepath.Join(binDir, "caddy"), BinaryName: "caddy", Hints: []string{"caddy", "linux", arch, ".tar.gz"}},
		// snell-v5 upstream release distribution is not publicly standardized.
		{Name: "snell-v5", Repo: "", BinaryPath: filepath.Join(binDir, "snell-server"), BinaryName: "snell-server", Hints: nil},
	}
}

func selectCoreAsset(meta coreComponent, assets []gh.ReleaseAsset) (gh.ReleaseAsset, error) {
	if len(meta.Hints) == 0 {
		return gh.ReleaseAsset{}, fmt.Errorf("no asset hints configured for %s", meta.Name)
	}
	return update.SelectAssetForRuntime(assets, meta.Hints...)
}

func isTarArchive(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz")
}

func isZipArchive(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".zip")
}

func replaceCoreBinary(targetPath, sourceBinary string) (string, error) {
	if strings.TrimSpace(targetPath) == "" || strings.TrimSpace(sourceBinary) == "" {
		return "", fmt.Errorf("invalid target/source")
	}
	// Only backup if the existing binary exists (skip for fresh installs).
	var backup string
	if _, err := os.Stat(targetPath); err == nil {
		backup, _ = fileutil.BackupFile(targetPath, filepath.Dir(targetPath))
	}
	content, err := os.ReadFile(sourceBinary)
	if err != nil {
		return backup, err
	}
	if err := fileutil.AtomicWrite(targetPath, content, 0o755); err != nil {
		return backup, err
	}
	return backup, nil
}
