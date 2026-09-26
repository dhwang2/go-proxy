package cert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
)

func TestCaddyfileRetainsConfiguredTLSNames(t *testing.T) {
	dir := t.TempDir()
	oldConfig, oldCaddy, oldSite := config.SingBoxConfig, config.CaddyFile, config.CaddySiteDir
	config.SingBoxConfig = filepath.Join(dir, "sing-box.json")
	config.CaddyFile = filepath.Join(dir, "Caddyfile")
	config.CaddySiteDir = filepath.Join(dir, "site")
	t.Cleanup(func() { config.SingBoxConfig = oldConfig; config.CaddyFile = oldCaddy; config.CaddySiteDir = oldSite })
	input := `{"inbounds":[
  {"type":"vless","tls":{"enabled":true,"server_name":"old.example.org"}},
  {"type":"tuic","tls":{"enabled":true,"server_name":"OLD.EXAMPLE.ORG"}},
  {"type":"anytls","tls":{"enabled":true,"server_name":"new.example.org"}},
  {"type":"vless","tls":{"enabled":true,"server_name":"reality.example.org","reality":{"enabled":true}}},
  {"type":"vless","tls":{"enabled":false,"server_name":"disabled.example.org"}}
 ]}`
	if err := os.WriteFile(config.SingBoxConfig, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateCaddyfile("new.example.org", ""); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(config.CaddyFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old.example.org:18443", "new.example.org:18443"} {
		if strings.Count(string(content), name) != 1 {
			t.Fatalf("certificate domain missing or duplicated: %s", name)
		}
	}
	for _, name := range []string{"reality.example.org", "disabled.example.org"} {
		if strings.Contains(string(content), name) {
			t.Fatalf("unexpected certificate request for %s", name)
		}
	}
	if err := os.WriteFile(config.SingBoxConfig, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateCaddyfile("another.example.org", ""); err == nil {
		t.Fatal("accepted unreadable configured domains")
	}
	after, err := os.ReadFile(config.CaddyFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(content) {
		t.Fatal("invalid input replaced the existing caddy configuration")
	}
}

// A regenerated Caddyfile stays on the port the site was moved to and keeps
// the ACME contact; the page is written once and an operator's own page
// survives. Only a site on 443 redirects plain HTTP.
func TestCaddyfileKeepsPortContactAndPage(t *testing.T) {
	dir := t.TempDir()
	oldConfig, oldCaddy, oldSite := config.SingBoxConfig, config.CaddyFile, config.CaddySiteDir
	config.SingBoxConfig = filepath.Join(dir, "sing-box.json")
	config.CaddyFile = filepath.Join(dir, "Caddyfile")
	config.CaddySiteDir = filepath.Join(dir, "site")
	t.Cleanup(func() { config.SingBoxConfig = oldConfig; config.CaddyFile = oldCaddy; config.CaddySiteDir = oldSite })
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{"inbounds":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateCaddyfile("a.example.org", "ops@example.org"); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(config.CaddyFile)
	for _, want := range []string{"a.example.org:18443 {", "email ops@example.org", "auto_https disable_redirects", "protocols h1 h2", "file_server"} {
		if !strings.Contains(string(first), want) {
			t.Fatalf("first Caddyfile lacks %q:\n%s", want, first)
		}
	}
	index := filepath.Join(config.CaddySiteDir, "index.html")
	if page, err := os.ReadFile(index); err != nil || !strings.Contains(string(page), "<title>a.example.org</title>") {
		t.Fatalf("placeholder page: %q %v", page, err)
	}
	if err := os.WriteFile(index, []byte("mine"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.CaddyFile, []byte(strings.ReplaceAll(string(first), ":18443", ":443")), 0644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateCaddyfile("a.example.org", ""); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(config.CaddyFile)
	if !strings.Contains(string(again), "a.example.org:443 {") || !strings.Contains(string(again), "email ops@example.org") {
		t.Fatalf("regenerated Caddyfile lost the port or the contact:\n%s", again)
	}
	if strings.Contains(string(again), "disable_redirects") {
		t.Fatalf("a site on 443 does not redirect plain http:\n%s", again)
	}
	if page, _ := os.ReadFile(index); string(page) != "mine" {
		t.Fatalf("the operator's page was replaced: %q", page)
	}
}
