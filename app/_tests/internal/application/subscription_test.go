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
	result, err := a.Subscription(ctx, SubscriptionOptions{Target: "ip", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("machine export must be handed to the caller encoded: %T", result.Data)
	}
	// Not null: a client reading the envelope has to be able to iterate
	// targets without first testing it for a JSON literal.
	if string(data) != `{"nodes":[],"targets":{"links":[]}}` {
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
	result, err = a.Subscription(ctx, SubscriptionOptions{Target: "ip", JSON: true})
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
		Targets subscription.Targets `json:"targets"`
	}
	if err := decoder.Decode(&export); err != nil {
		t.Fatalf("invalid export: %v: %s", err, data)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("export must be exactly one JSON value: %v", err)
	}
	if len(export.Nodes) != 2 || len(export.Targets.Links) != 1 || export.Targets.Links[0].Host != "192.0.2.1" {
		t.Fatalf("export selection: %s", data)
	}
	renderer := subscription.NewRenderer(snapshot.Store, snapshot.Bindings, export.Targets)
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
	// The export carries no links at all rather than a nil result, which the
	// entry point would have rendered as null.
	plain, err := a.Subscription(ctx, SubscriptionOptions{Target: "ip"})
	if err != nil {
		t.Fatal(err)
	}
	links, ok := plain.Data.(map[string]any)["links"].([]SubscriptionLink)
	if !ok {
		t.Fatalf("export is not a link list: %#v", plain.Data)
	}
	if len(links) != 0 {
		t.Fatalf("a user with no nodes exported %d links", len(links))
	}
}

// exportLinks is the link list an export hands the renderer.
func exportLinks(t *testing.T, result Result) []SubscriptionLink {
	t.Helper()
	links, ok := result.Data.(map[string]any)["links"].([]SubscriptionLink)
	if !ok {
		t.Fatalf("export is not a link list: %#v", result.Data)
	}
	return links
}

// linksIn is the content of the links of one format.
func linksIn(links []SubscriptionLink, format string) []string {
	var contents []string
	for _, link := range links {
		if link.Format == format {
			contents = append(contents, link.Content)
		}
	}
	return contents
}

// A client that cannot represent one node should still get the others. The
// export used to refuse the whole request, which sent a Surge reader away with
// nothing over a node they never asked about.
func TestEachNodeExportsTheFormatsItHas(t *testing.T) {
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
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "ip"})
	if err != nil {
		t.Fatal(err)
	}
	// Surge cannot carry vless; the anytls node is still in its section.
	surge := strings.Join(linksIn(exportLinks(t, result), "surge"), "\n")
	if !strings.Contains(surge, "= anytls,") || strings.Contains(surge, "vless") {
		t.Fatalf("surge section: %q", surge)
	}
}

// Each mihomo entry is one flow mapping with what the client needs to connect,
// and never the key that stays on the server.
func TestMihomoLinksCarryWhatTheClientNeeds(t *testing.T) {
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
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "ip"})
	if err != nil {
		t.Fatal(err)
	}
	entries := linksIn(exportLinks(t, result), "mihomo")
	if len(entries) != 2 {
		t.Fatalf("expected one mihomo entry per node: %q", entries)
	}
	for _, entry := range entries {
		// Flow style: one mapping on one line.
		if !strings.HasPrefix(entry, "{") || !strings.HasSuffix(entry, "}") {
			t.Fatalf("entry is not one mapping: %q", entry)
		}
		for _, want := range []string{`name: "`, `type: "`, `server: "192.0.2.1"`, "port: "} {
			if !strings.Contains(entry, want) {
				t.Fatalf("entry is missing %q: %s", want, entry)
			}
		}
	}
	// The reality entry carries what the handshake needs, and never the key
	// that stays on the server.
	all := strings.Join(entries, "\n")
	if !strings.Contains(all, "reality-opts: {public-key:") {
		t.Fatalf("reality node exported without its handshake key: %s", all)
	}
	if strings.Contains(all, snapshot.Store.SingBox.Inbounds[0].TLS.Reality.PrivateKey) {
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
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "ip"})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(linksIn(exportLinks(t, result), "mihomo"), "\n")
	seen := map[string]bool{}
	for _, name := range regexp.MustCompile(`name: "([^"]+)"`).FindAllStringSubmatch(all, -1) {
		if seen[name[1]] {
			t.Fatalf("duplicate proxy name %q:\n%s", name[1], all)
		}
		seen[name[1]] = true
	}
	if len(seen) == 0 {
		t.Fatalf("no named entries: %s", all)
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
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "ip"})
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

// A user's export is that user's links and no one else's; without --user it
// is every user's.
func TestSubscriptionIsScopedToTheUser(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []protocol.InstallParams{
		{ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"},
		{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "bob", SNI: "www.netbsd.org"},
	} {
		if _, err := protocol.Install(snapshot.Store, p); err != nil {
			t.Fatal(err)
		}
	}
	for i := range snapshot.Store.SingBox.Inbounds {
		if ib := &snapshot.Store.SingBox.Inbounds[i]; ib.Tag == "anytls_24445" {
			if _, err := protocol.AddUserToExisting(snapshot.Store, ib, "bob"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	users := func(opts SubscriptionOptions) (map[string]bool, map[string]bool) {
		result, err := a.Subscription(ctx, opts)
		if err != nil {
			t.Fatal(err)
		}
		who, tags := map[string]bool{}, map[string]bool{}
		for _, link := range exportLinks(t, result) {
			who[link.User], tags[link.Tag] = true, true
		}
		return who, tags
	}
	if who, tags := users(SubscriptionOptions{User: "alice", Target: "ip"}); len(who) != 1 || !who["alice"] || tags["vless_reality_24443"] {
		t.Fatalf("alice's export: users %v nodes %v", who, tags)
	}
	if who, _ := users(SubscriptionOptions{Target: "ip"}); len(who) != 2 {
		t.Fatalf("every user's export: users %v", who)
	}
}

// --target is domain or ip. A domain export on a host with no domain names
// the server's addresses rather than failing, and says so.
func TestSubscriptionTargetModes(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	if _, err := a.Subscription(ctx, SubscriptionOptions{Target: "192.0.2.1"}); err == nil || !strings.Contains(err.Error(), "domain or ip") {
		t.Fatalf("an address was accepted as a target mode: %v", err)
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Install(snapshot.Store, protocol.InstallParams{ProtoType: protocol.AnyTLS, Port: 24445, UserName: "alice", Domain: "example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}
	var notes []string
	a.Progress = func(line string) { notes = append(notes, line) }
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range linksIn(exportLinks(t, result), "surge") {
		if !strings.Contains(content, "192.0.2.1") {
			t.Fatalf("domain export without a domain did not use the address: %s", content)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "no domain is configured") {
		t.Fatalf("notes: %q", notes)
	}
}
