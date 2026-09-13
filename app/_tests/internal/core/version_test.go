package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSnellVersionFromStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snell-server")
	script := "#!/bin/sh\nprintf '%s\\n' '2026-09-13 23:52:43.440803 [server_main] <NOTIFY> snell-server v6.0.0 (Aug 7 2026)' >&2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	info := DetectVersion(context.Background(), path, CompSnell)
	if !info.Installed || info.Version != "v6.0.0" {
		t.Fatalf("unexpected Snell version: %+v", info)
	}
}
