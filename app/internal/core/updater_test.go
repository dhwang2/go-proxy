package core

import (
	"os"
	"path/filepath"
	"testing"

	gh "github.com/dhwang2/go-proxy/pkg/github"
)

func TestSelectCoreAsset(t *testing.T) {
	meta := coreComponent{Name: "sing-box", Hints: []string{"sing-box-linux-amd64"}}
	assets := []gh.ReleaseAsset{{Name: "sing-box-linux-amd64", DownloadURL: "https://example.com/sing-box"}}
	asset, err := selectCoreAsset(meta, assets)
	if err != nil {
		t.Fatalf("selectCoreAsset error: %v", err)
	}
	if asset.Name != "sing-box-linux-amd64" {
		t.Fatalf("unexpected asset name: %s", asset.Name)
	}
}

func TestReplaceCoreBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target-bin")
	source := filepath.Join(dir, "source-bin")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup, err := replaceCoreBinary(target, source)
	if err != nil {
		t.Fatalf("replaceCoreBinary error: %v", err)
	}
	if backup == "" {
		t.Fatalf("expected backup path")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Fatalf("unexpected target content: %s", string(content))
	}
}
