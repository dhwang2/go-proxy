package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoutingNetworkRejectInvalidInputBeforeRuntime(t *testing.T) {
	cases := [][]string{
		{"routing", "set", "alice", "--preset", "unknown", "--outbound", "direct"},
		{"routing", "remove", "alice", "--rules", "0", "--yes"},
		{"routing", "remove", "alice", "--rules", "1"},
		{"routing", "clear", "alice", "--all", "--yes"},
		{"routing", "clear", "--yes"},
		{"routing", "direct", "--strategy", "unknown"},
		{"routing", "chain", "add", "relay", "--host", "example.com", "--port", "0"},
		{"routing", "chain", "add", "relay", "--host", "example.com", "--port", "1080", "--credentials-file", "-"},
		{"network", "firewall", "add", "65536", "--transport", "both"},
		{"network", "firewall", "add", "80", "--transport", "icmp"},
		{"network", "firewall", "remove", "80", "--transport", "tcp"},
		{"network", "firewall", "clear"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			r := New("test", "test", strings.NewReader(`{"username":"private-user","password":"private-password","unknown":true}`), &out, &stderr)
			r.App.LockDir = filepath.Join(t.TempDir(), "uninitialized")
			code := r.Run(context.Background(), append(args, "--json"))
			if code != 2 {
				t.Fatalf("exit %d, want 2; stdout=%s stderr=%s", code, out.String(), stderr.String())
			}
			var response struct {
				OK      bool `json:"ok"`
				Changed bool `json:"changed"`
			}
			if err := json.Unmarshal(out.Bytes(), &response); err != nil || response.OK || response.Changed {
				t.Fatalf("invalid error envelope: %s (%v)", out.String(), err)
			}
			if strings.Contains(out.String()+stderr.String(), "private-") {
				t.Fatal("credentials leaked through invalid argument output")
			}
		})
	}
}

func TestChainCredentialsBoundInputAndCancellation(t *testing.T) {
	args := []string{"routing", "chain", "add", "relay", "--host", "example.com", "--port", "1080", "--credentials-file", "-", "--json"}
	var out, stderr bytes.Buffer
	oversized := `{"username":"private-user","password":"private-password"}` + strings.Repeat(" ", 65536) + `{}`
	r := New("test", "test", strings.NewReader(oversized), &out, &stderr)
	if code := r.Run(context.Background(), args); code != 2 {
		t.Fatalf("oversized input exit %d: %s", code, out.String())
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	out.Reset()
	stderr.Reset()
	r = New("test", "test", reader, &out, &stderr)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	if code := r.Run(ctx, args); code != 1 {
		t.Fatalf("deadline exit %d: %s", code, out.String())
	}
	if time.Since(started) > time.Second {
		t.Fatal("blocked credentials input ignored cancellation")
	}
}
