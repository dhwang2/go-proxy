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

// sing-box and caddy colour their own logs. The reader strips that, because
// --json promises to carry no escape sequence under any condition and the JSON
// path never reaches a renderer that could strip it later.
func TestReadStripsForeignTerminalControls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	body := "\x1b[33mWARN\x1b[0m inbound started\nplain line\n\x1b[31mERROR\x1b[0m gone\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	content, _, err := Read(context.Background(), path, "unused", 10, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(content, 0x1b) {
		t.Fatalf("log content kept an escape sequence: %q", content)
	}
	if content != "WARN inbound started\nplain line\nERROR gone\n" {
		t.Fatalf("stripping changed the log text: %q", content)
	}
}
