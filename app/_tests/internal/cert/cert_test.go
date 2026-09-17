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
	oldConfig, oldCaddy := config.SingBoxConfig, config.CaddyFile
	config.SingBoxConfig = filepath.Join(dir, "sing-box.json")
	config.CaddyFile = filepath.Join(dir, "Caddyfile")
	t.Cleanup(func() { config.SingBoxConfig = oldConfig; config.CaddyFile = oldCaddy })
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
