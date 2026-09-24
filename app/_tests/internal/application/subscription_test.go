package application

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"regexp"
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
	// Not null: a client reading the envelope has to be able to iterate
	// targets without first testing it for a JSON literal.
	if string(data) != `{"nodes":[],"targets":[]}` {
		t.Fatalf("empty export: %s", data)
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []protocol.InstallParams{{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"}, {ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"}} {
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
		`{"type":"anytls","password":"a/b+c=="}`, "<script>&amp;</script>",
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

// A user with no nodes exports nothing. It used to export the four characters
// "null": the renderer left Raw nil, the entry point read that as "produced no
// export" and printed the nil result through the JSON fallback instead. Surge
// and a URI list cannot read that, and neither can a person.
func TestEmptySubscriptionExportsNothingRatherThanNull(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	// The default export is structured rather than raw now: it is grouped for
	// display, so it is checked separately below.
	for _, format := range []subscription.Format{subscription.FormatSurge, subscription.FormatURI} {
		result, err := a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1", Format: format})
		if err != nil {
			t.Fatalf("format %q: %v", format, err)
		}
		if result.Raw == nil {
			t.Fatalf("format %q handed back a nil export, which the entry point renders as null", format)
		}
		if len(result.Raw) != 0 {
			t.Fatalf("format %q exported %q for a user with no nodes", format, result.Raw)
		}
	}
	// The default export carries no links at all rather than a nil result,
	// which the entry point would have rendered as null.
	plain, err := a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	links, ok := plain.Data.(map[string]any)["links"].([]SubscriptionLink)
	if !ok {
		t.Fatalf("default export is not a link list: %#v", plain.Data)
	}
	if len(links) != 0 {
		t.Fatalf("a user with no nodes exported %d links", len(links))
	}

	// Asked for by name, mihomo is the document a client loads, so an empty
	// one is a well-formed empty list rather than a key with nothing under it.
	result, err := a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1", Format: subscription.FormatMihomo})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(result.Raw)) != "proxies: []" {
		t.Fatalf("empty mihomo export is %q", result.Raw)
	}
}

// A client that cannot represent one node should still get the others. The
// export used to refuse the whole request, which sent a Surge reader away with
// nothing over a node they never asked about.
func TestUnsupportedNodesAreSkippedRatherThanRefused(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []protocol.InstallParams{
		{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"},
		{ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"},
	} {
		if _, err := protocol.Install(snapshot.Store, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}

	// Surge cannot carry vless; the anytls node still exports.
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1", Format: subscription.FormatSurge})
	if err != nil {
		t.Fatalf("mixed surge export failed instead of skipping: %v", err)
	}
	body := string(result.Raw)
	if !strings.Contains(body, "= anytls,") {
		t.Fatalf("the supported node was dropped with the unsupported one: %q", body)
	}
	if strings.Contains(body, "vless") {
		t.Fatalf("a node surge cannot represent reached the output: %q", body)
	}

	// Naming the node and the format together is an explicit pair.
	if _, err := a.Subscription(ctx, SubscriptionOptions{
		User: "alice", Node: "vless_reality_24443", Target: "192.0.2.1", Format: subscription.FormatSurge,
	}); err == nil {
		t.Fatal("an explicitly selected node with no surge export must fail")
	}
}

// Asked for by name, the mihomo export is the document a client loads: a
// proxies key with sequence items under it, well-formed even when empty.
func TestMihomoExportIsAProxiesDocument(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	empty, err := a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1", Format: subscription.FormatMihomo})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(empty.Raw)) != "proxies: []" {
		t.Fatalf("empty mihomo export is %q", empty.Raw)
	}

	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []protocol.InstallParams{
		{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"},
		{ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"},
	} {
		if _, err := protocol.Install(snapshot.Store, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1", Format: subscription.FormatMihomo})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(result.Raw), "\n"), "\n")
	if lines[0] != "proxies:" {
		t.Fatalf("export does not open the list: %q", result.Raw)
	}
	if len(lines) != 3 {
		t.Fatalf("expected one entry per node, got %d lines: %s", len(lines)-1, result.Raw)
	}
	for _, line := range lines[1:] {
		// Flow style: two spaces, a dash, and one mapping on one line.
		if !strings.HasPrefix(line, "  - {") || !strings.HasSuffix(line, "}") {
			t.Fatalf("entry is not a sequence item: %q", line)
		}
		for _, want := range []string{`name: "`, `type: "`, `server: "192.0.2.1"`, "port: "} {
			if !strings.Contains(line, want) {
				t.Fatalf("entry is missing %q: %s", want, line)
			}
		}
	}
	// The reality entry carries what the handshake needs, and never the key
	// that stays on the server.
	if !strings.Contains(string(result.Raw), "reality-opts: {public-key:") {
		t.Fatalf("reality node exported without its handshake key: %s", result.Raw)
	}
	if strings.Contains(string(result.Raw), snapshot.Store.SingBox.Inbounds[0].TLS.Reality.PrivateKey) {
		t.Fatal("client export contains a server private key")
	}
}

// Every mihomo entry has to be a distinct proxy: the client keys its proxy
// list by name, so two nodes sharing one would silently replace each other.
func TestMihomoEntriesAreUniquelyNamed(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []protocol.InstallParams{
		{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"},
		{ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"},
	} {
		if _, err := protocol.Install(snapshot.Store, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1", Format: subscription.FormatMihomo})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, name := range regexp.MustCompile(`name: "([^"]+)"`).FindAllStringSubmatch(string(result.Raw), -1) {
		if seen[name[1]] {
			t.Fatalf("duplicate proxy name %q:\n%s", name[1], result.Raw)
		}
		seen[name[1]] = true
	}
	if len(seen) == 0 {
		t.Fatalf("no named entries: %s", result.Raw)
	}
}

// The default export hands back every link with enough about it to be grouped,
// ordered format first, then user, then address family so the families under
// one user sit together.
func TestDefaultExportIsOrderedForGrouping(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []protocol.InstallParams{
		{ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"},
		{ProtoType: protocol.TUIC, Port: 24446, UserName: "alice", Domain: "example.com"},
	} {
		if _, err := protocol.Install(snapshot.Store, params); err != nil {
			t.Fatal(err)
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	links := result.Data.(map[string]any)["links"].([]SubscriptionLink)
	if len(links) == 0 {
		t.Fatal("no links")
	}
	var previous SubscriptionLink
	for index, link := range links {
		if link.Format == "" || link.User == "" || link.Tag == "" || link.Content == "" {
			t.Fatalf("link %d is missing what it needs to be grouped: %#v", index, link)
		}
		if index > 0 {
			before := []string{previous.Format, previous.User, previous.Family, previous.Tag}
			after := []string{link.Format, link.User, link.Family, link.Tag}
			if strings.Join(before, "\x00") > strings.Join(after, "\x00") {
				t.Fatalf("links are out of grouping order at %d: %#v then %#v", index, previous, link)
			}
		}
		previous = link
	}
}
