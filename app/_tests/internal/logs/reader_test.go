package logs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileBoundsAndRequestedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	if err := os.WriteFile(path, []byte("first\nsecond\nthird\n"), 0600); err != nil {
		t.Fatal(err)
	}
	content, source, err := Read(context.Background(), path, "unused", 2, 1024)
	if err != nil || source != path || content != "second\nthird\n" {
		t.Fatalf("unexpected log result %q %q %v", content, source, err)
	}
	if _, _, err := Read(context.Background(), path, "unused", 3, 4); !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("wanted output limit, got %v", err)
	}
}
func TestFollowCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	if err := os.WriteFile(path, []byte("entry\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out strings.Builder
	if err := Follow(ctx, path, "unused", 1, &out); err == nil {
		t.Fatal("cancelled follow succeeded")
	}
}
