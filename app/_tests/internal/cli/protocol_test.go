package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtocolAndSubscriptionUsageErrorsAreNonInteractive(t *testing.T) {
	for _, args := range [][]string{
		{"protocol", "add", "ss", "--port", "auto"},
		{"protocol", "add", "ss", "--user", "alice"},
		{"protocol", "add", "trojan", "--user", "alice", "--port", "auto"},
		{"protocol", "add", "ss", "--user", "alice", "--port", "auto", "--reality"},
		{"protocol", "add", "vless", "--user", "alice", "--port", "auto", "--reality", "--domain", "example.com"},
		{"protocol", "add", "vless", "--user", "alice", "--port", "auto", "--reality", "--sni", "www.apple.com"},
		{"protocol", "add", "ss", "--user", "alice", "--port", "auto", "--shadow-tls-sni", "www.kernel.org"},
		{"protocol", "remove", "some-node"},
		{"protocol", "remove", "some-node", "--yes", "--user", ""},
		{"user", "delete", "alice"},
		{"sub", "--sing-box"},
		{"sub", "--singbox"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := New("test", "test", strings.NewReader(""), &out, &stderr)
			r.App.RequireRoot = false
			r.App.LockDir = filepath.Join(t.TempDir(), "absent")
			code := r.Run(context.Background(), append(args, "--json"))
			if code != 2 {
				t.Fatalf("exit=%d out=%s err=%s", code, out.String(), stderr.String())
			}
			var value struct {
				OK      bool `json:"ok"`
				Changed bool `json:"changed"`
				Error   struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &value); err != nil || value.OK || value.Changed || value.Error.Code != "invalid_argument" {
				t.Fatalf("invalid failure envelope %s (%v)", out.String(), err)
			}
			if _, err := os.Stat(r.App.LockDir); !os.IsNotExist(err) {
				t.Fatal("usage error created runtime state")
			}
		})
	}
}

func TestProtocolHelpRequiresNoRuntime(t *testing.T) {
	var out, stderr bytes.Buffer
	r := New("test", "test", strings.NewReader(""), &out, &stderr)
	r.App.LockDir = filepath.Join(t.TempDir(), "absent")
	if code := r.Run(context.Background(), []string{"protocol", "add", "--help"}); code != 0 {
		t.Fatalf("help exit=%d stderr=%s", code, stderr.String())
	}
	for _, flag := range []string{"--reality", "--shadow-tls", "--port", "--user"} {
		if !strings.Contains(out.String(), flag) {
			t.Fatalf("help omitted %s", flag)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help stderr=%s", stderr.String())
	}
}
