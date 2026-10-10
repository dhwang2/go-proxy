package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// An update rewrites the completion scripts the installer placed, from the new
// executable, and leaves a shell without one alone.
func TestRefreshCompletionsRewritesInstalledScripts(t *testing.T) {
	dir := t.TempDir()
	bash, zsh := filepath.Join(dir, "gproxy"), filepath.Join(dir, "_gproxy")
	if err := os.WriteFile(bash, []byte("complete -o default -F __start_gproxy gproxy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "gproxy-new")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\necho \"script for $2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RefreshCompletions(context.Background(), executable, bash, zsh); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(bash)
	if err != nil || string(data) != "script for bash\n" {
		t.Fatalf("bash script %q, %v", data, err)
	}
	if info, _ := os.Stat(bash); info.Mode().Perm() != 0o644 {
		t.Fatalf("bash script mode %v, want 0644 so every user's shell can read it", info.Mode().Perm())
	}
	if _, err := os.Stat(zsh); !os.IsNotExist(err) {
		t.Fatalf("an absent zsh script was created: %v", err)
	}

	failing := filepath.Join(dir, "gproxy-broken")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RefreshCompletions(context.Background(), failing, bash, zsh); err == nil {
		t.Fatal("a failed generation was reported as refreshed")
	}
	if data, _ := os.ReadFile(bash); string(data) != "script for bash\n" {
		t.Fatalf("a failed generation replaced the script: %q", data)
	}
}
