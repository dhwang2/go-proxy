package store

import (
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
)

func TestReadCaddySiteReadsPortAndEmail(t *testing.T) {
	saved := config.CaddyFile
	config.CaddyFile = filepath.Join(t.TempDir(), "Caddyfile")
	t.Cleanup(func() { config.CaddyFile = saved })
	if _, found := ReadCaddySite(); found {
		t.Fatal("a missing Caddyfile was found")
	}
	for content, want := range map[string]CaddySite{
		"{\n    email ops@example.org\n}\n\na.example.org:443, b.example.org:443 {\n    file_server\n}\n": {Port: 443, Email: "ops@example.org"},
		"{\n    auto_https disable_redirects\n}\n\na.example.org:18443 {\n    respond \"ok\" 200\n}\n":    {Port: 18443},
		"{\n}\n\na.example.org {\n}\n": {Port: DefaultCaddyPort},
	} {
		if err := os.WriteFile(config.CaddyFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if site, found := ReadCaddySite(); !found || site != want {
			t.Fatalf("read %#v from %q, want %#v", site, content, want)
		}
	}
}
