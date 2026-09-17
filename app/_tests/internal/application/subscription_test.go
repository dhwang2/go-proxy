package application

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"go-proxy/internal/protocol"
	"go-proxy/internal/subscription"
)

func TestSubscriptionMachineExportIsOneEncodedValue(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	result, err := a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("machine export must be handed to the caller encoded: %T", result.Data)
	}
	if string(data) != `{"nodes":[],"targets":null}` {
		t.Fatalf("empty export: %s", data)
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []protocol.InstallParams{{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"}, {ProtoType: protocol.Shadowsocks, Port: 24445, UserName: "alice"}} {
		if _, err := protocol.Install(snapshot.Store, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	result, err = a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	data, ok = result.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("machine export must be handed to the caller encoded: %T", result.Data)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var export struct {
		Nodes []struct {
			Tag      string                           `json:"tag"`
			User     string                           `json:"user"`
			Protocol string                           `json:"protocol"`
			Port     int                              `json:"port"`
			Formats  []subscription.Format            `json:"formats"`
			Content  map[subscription.Format][]string `json:"content"`
		} `json:"nodes"`
		Targets []subscription.SurgeTarget `json:"targets"`
	}
	if err := decoder.Decode(&export); err != nil {
		t.Fatalf("invalid export: %v: %s", err, data)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("export must be exactly one JSON value: %v", err)
	}
	if len(export.Nodes) != 2 || len(export.Targets) != 1 || export.Targets[0].Host != "192.0.2.1" {
		t.Fatalf("export selection: %s", data)
	}
	renderer := subscription.NewRenderer(snapshot.Store, snapshot.Bindings, "", export.Targets)
	for _, node := range export.Nodes {
		if node.User != "alice" || node.Port == 0 || len(node.Formats) == 0 {
			t.Fatalf("node fields: %#v", node)
		}
		for _, format := range node.Formats {
			content := node.Content[format]
			if len(content) != 1 {
				t.Fatalf("node %q omitted %s content: %#v", node.Tag, format, node.Content)
			}
			if strings.Contains(content[0], "\n") || content[0] == "" {
				t.Fatalf("node %q rendered an unusable %s link", node.Tag, format)
			}
		}
		if len(node.Content) != len(node.Formats) {
			t.Fatalf("node %q exported unexpected formats: %#v", node.Tag, node.Content)
		}
		_ = renderer
	}
	if bytes.Contains(data, []byte(snapshot.Store.SingBox.Inbounds[0].TLS.Reality.PrivateKey)) {
		t.Fatal("machine export contains a server private key")
	}
}

func TestSubscriptionStringsEncodeLikeEncodingJSON(t *testing.T) {
	for _, value := range []string{
		"", "plain", `vless://id@192.0.2.1:443?security=reality&sni=a.example#alice / node`,
		`{"type":"shadowsocks","password":"a/b+c=="}`, "<script>&amp;</script>",
		"tab\tnewline\nreturn\rquote\"backslash\\", "\x00\x01\x1f\x7f", "bell\bform\f",
		"héllo 世界   ", "emoji 🙂",
		// encoding/json escapes these because they break JSONP; the encoder mirrors that.
		"line\u2028separator", "paragraph\u2029separator", "\u2027 neighbour \u202a",
	} {
		want, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if got := appendJSONString(nil, value); !bytes.Equal(got, want) {
			t.Fatalf("encoded %q as %s, want %s", value, got, want)
		}
	}
}

// Invalid UTF-8 is rendered differently by different encoding/json releases: some
// escape the replacement rune, others emit it literally. Both decode to the same
// string, so assert the decoded value rather than the bytes.
func TestSubscriptionInvalidUTF8DecodesLikeEncodingJSON(t *testing.T) {
	for _, value := range []string{"invalid \xff\xfe utf8", "\xf0\x28\x8c\x28", "trailing \xe2\x82"} {
		want, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		got := appendJSONString(nil, value)
		var fromGot, fromWant string
		if err := json.Unmarshal(got, &fromGot); err != nil {
			t.Fatalf("encoder produced invalid JSON for %q: %s: %v", value, got, err)
		}
		if err := json.Unmarshal(want, &fromWant); err != nil {
			t.Fatal(err)
		}
		if fromGot != fromWant {
			t.Fatalf("decoded %q as %q, want %q", value, fromGot, fromWant)
		}
	}
}
