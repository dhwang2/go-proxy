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
	content, source, err := Read(context.Background(), path, "unused", 2)
	if err != nil || source != path || content != "second\nthird\n" {
		t.Fatalf("unexpected log result %q %q %v", content, source, err)
	}
	// A read is bounded at 1 MiB whatever --lines asks for.
	long := strings.Repeat("x", 700<<10) + "\n"
	if err := os.WriteFile(path, []byte(long+long), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(context.Background(), path, "unused", 2); !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("wanted output limit, got %v", err)
	}
}

// The zone offset sing-box puts before its timestamp and the seconds counter
// in its startup lines carry nothing, and are dropped; other lines are kept
// as written.
func TestReadDropsSingBoxsOffsetAndStartupCounter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	body := "+0000 2026-09-29 06:56:33 ERROR [196379889 135ms] inbound/anytls[anytls_2053]: EOF\n" +
		"\x1b[33mWARN\x1b[0m[0000] `independent_cache` DNS option is deprecated\n" +
		"2026-09-27 09:19:00.481964 [server_tunnel-1] <WARN> Session error E01\n" +
		"price +0000 2026-09-29 stays\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	content, _, err := Read(context.Background(), path, "unused", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-09-29 06:56:33 ERROR [196379889 135ms] inbound/anytls[anytls_2053]: EOF\n" +
		"WARN `independent_cache` DNS option is deprecated\n" +
		"2026-09-27 09:19:00.481964 [server_tunnel-1] <WARN> Session error E01\n" +
		"price +0000 2026-09-29 stays\n"
	if content != want {
		t.Fatalf("got:\n%s\nwant:\n%s", content, want)
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
	content, _, err := Read(context.Background(), path, "unused", 10)
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
