package subscription

import (
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
)

func TestDetectTargetPrefersEnv(t *testing.T) {
	dir := t.TempDir()
	prev := config.DomainFile
	config.DomainFile = filepath.Join(dir, ".domain")
	defer func() { config.DomainFile = prev }()

	if err := os.WriteFile(config.DomainFile, []byte("example.com\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("PROXY_HOST", "1.2.3.4")

	if got := DetectTarget(); got != "1.2.3.4" {
		t.Fatalf("DetectTarget() = %q, want %q", got, "1.2.3.4")
	}
}

func TestDetectTargetUsesStoredDomain(t *testing.T) {
	dir := t.TempDir()
	prev := config.DomainFile
	config.DomainFile = filepath.Join(dir, ".domain")
	defer func() { config.DomainFile = prev }()

	if err := os.WriteFile(config.DomainFile, []byte("sub.example.com\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("PROXY_HOST", "")

	if got := DetectTarget(); got != "sub.example.com" {
		t.Fatalf("DetectTarget() = %q, want %q", got, "sub.example.com")
	}
}
