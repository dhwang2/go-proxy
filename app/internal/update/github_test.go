package update

import (
	"testing"

	gh "github.com/dhwang2/go-proxy/pkg/github"
)

func TestCompareTags(t *testing.T) {
	if got := CompareTags("v1.2.3", "v1.2.4"); got != CompareLess {
		t.Fatalf("expected CompareLess, got %v", got)
	}
	if got := CompareTags("1.3.0", "v1.2.9"); got != CompareGreater {
		t.Fatalf("expected CompareGreater, got %v", got)
	}
	if got := CompareTags("v1.2.3", "1.2.3"); got != CompareEqual {
		t.Fatalf("expected CompareEqual, got %v", got)
	}
	if got := CompareTags("v0.0.0-dev", "go-proxy-v0.0.1"); got != CompareLess {
		t.Fatalf("expected CompareLess for prefixed tag, got %v", got)
	}
	if got := CompareTags("go-proxy-v0.0.1", "v0.0.1"); got != CompareEqual {
		t.Fatalf("expected CompareEqual for prefixed tag, got %v", got)
	}
}

func TestSelectAssetForRuntime(t *testing.T) {
	assets := []gh.ReleaseAsset{
		{Name: "proxy-linux-amd64", DownloadURL: "https://example.com/proxy-linux-amd64"},
		{Name: "proxy-arm64", DownloadURL: "https://example.com/proxy-arm64"},
	}
	asset, err := SelectAssetForRuntime(assets, "proxy-linux-amd64")
	if err != nil {
		t.Fatalf("SelectAssetForRuntime error: %v", err)
	}
	if asset.Name != "proxy-linux-amd64" {
		t.Fatalf("unexpected asset selected: %s", asset.Name)
	}
}
