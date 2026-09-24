package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

// AtomicWrite preserves the permissions a file already has, which is right for
// a configuration file it must not widen. A caller naming a mode is deciding
// them instead: the snell version receipt was left at 0600 beside a 0755
// binary, and an unprivileged `core check` could not read it.
func TestAtomicWriteModeForcesThePermissionsItIsGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snell-server.version")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("preserved\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("AtomicWrite changed the mode to %v", info.Mode().Perm())
	}
	if err := AtomicWriteMode(path, []byte("corrected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("AtomicWriteMode left the mode at %v", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "corrected\n" {
		t.Fatalf("content %q err %v", data, err)
	}
}
