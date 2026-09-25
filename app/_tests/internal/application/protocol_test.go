package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
	"go-proxy/internal/routing"
	"go-proxy/internal/store"
	"go-proxy/internal/subscription"
	"go-proxy/internal/user"
)

func protocolTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	for name, ptr := range map[string]*string{"sing-box.json": &config.SingBoxConfig, "users.json": &config.UserMetaFile, "routes.json": &config.UserRouteFile, "templates.json": &config.UserTemplateFile, "firewall.json": &config.FirewallConfigFile, "snell.conf": &config.SnellConfigFile, ".domain": &config.DomainFile} {
		old := *ptr
		*ptr = filepath.Join(dir, name)
		t.Cleanup(func() { *ptr = old })
	}
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	a := New(nil)
	a.LockDir = dir
	a.RequireRoot = false
	if _, err := a.Operation(context.Background(), func() (Result, error) { return Result{}, nil }); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRegisteredUserLifecycleNeedsNoCore(t *testing.T) {
	// Deleting a user reconciles the firewall. An absent nft is skipped gracefully,
	// but an nft that exists and cannot run (an unprivileged runner) is an error, so
	// keep this lifecycle test free of host tooling entirely.
	t.Setenv("PATH", t.TempDir())
	a := protocolTestApp(t)
	ctx := context.Background()
	result, err := a.UserAdd(ctx, "alice", false)
	if err != nil || !result.Changed {
		t.Fatalf("register: %#v %v", result, err)
	}
	result, err = a.UserAdd(ctx, "alice", false)
	if err != nil || result.Changed {
		t.Fatalf("repeat registration: %#v %v", result, err)
	}
	if _, err = a.UserRename(ctx, "alice", "bob"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := derived.UserNames(snapshot.Store); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("users: %v", got)
	}
	result, err = a.UserDelete(ctx, "bob")
	if err != nil || !result.Changed {
		t.Fatalf("delete: %#v %v", result, err)
	}
	result, err = a.UserDelete(ctx, "bob")
	if err != nil || result.Changed {
		t.Fatalf("repeat delete: %#v %v", result, err)
	}
}

func TestSubscriptionSelectionCapabilitiesAndRedaction(t *testing.T) {
	a := protocolTestApp(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := snapshot.Store
	for _, p := range []protocol.InstallParams{{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"}, {ProtoType: protocol.Snell, Port: 24444, UserName: "alice"}} {
		if _, err := protocol.Install(s, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// alice has a reality node sing-box can export and a snell node it cannot.
	// The export is the nodes that fit, not nothing: refusing the whole request
	// sent a reader away empty-handed over one node they never asked about.
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1", Format: subscription.FormatMihomo})
	if err != nil {
		t.Fatalf("mixed export failed instead of skipping: %v", err)
	}
	if strings.Count(string(result.Raw), "\n  - {") != 1 {
		t.Fatalf("mixed export did not carry the supported node alone: %s", result.Raw)
	}
	if strings.Contains(string(result.Raw), "snell") {
		t.Fatalf("a node with no sing-box export reached the output: %s", result.Raw)
	}
	// Naming one node and one format is an explicit pair; that still fails.
	if _, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Node: "snell-v6", Target: "192.0.2.1", Format: subscription.FormatMihomo}); err == nil {
		t.Fatal("an explicitly selected node with no export in the selected format must fail")
	}
	result, err = a.Subscription(ctx, SubscriptionOptions{User: "alice", Node: "vless_reality_24443", Target: "192.0.2.1", Format: subscription.FormatMihomo})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(result.Raw), "\n  - {") != 1 {
		t.Fatalf("single-node export: %s", result.Raw)
	}
	privateKey := s.SingBox.Inbounds[0].TLS.Reality.PrivateKey
	if strings.Contains(string(result.Raw), privateKey) {
		t.Fatal("client export contains a server private key")
	}
	result, err = a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1", JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.Data)
	if !strings.Contains(string(encoded), `"mihomo"`) || !strings.Contains(string(encoded), `"snell-v6"`) {
		t.Fatal("machine export omitted a format or node")
	}
	result, err = a.UserList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(result.Data)
	if strings.Contains(string(encoded), s.SnellConf.PSK) || strings.Contains(string(encoded), s.SingBox.Inbounds[0].Users[0].UUID) {
		t.Fatal("ordinary user query leaked credentials")
	}
	if _, err = a.Subscription(ctx, SubscriptionOptions{User: "missing", Target: "192.0.2.1"}); err == nil {
		t.Fatal("missing user silently exported another user")
	}
}

func TestProtocolArgumentsFailBeforeMutation(t *testing.T) {
	a := New(nil)
	a.LockDir = filepath.Join(t.TempDir(), "absent")
	a.RequireRoot = false
	for _, p := range []ProtocolOptions{
		{Type: protocol.AnyTLS, User: "alice", Port: "70000", Domain: "example.com"},
		{Type: protocol.VLESS, User: "alice", Port: "auto"},
		{Type: protocol.VLESSReality, User: "alice", Port: "auto", Domain: "example.com"},
		{Type: protocol.VLESSReality, User: "alice", Port: "auto", SNI: "www.apple.com"},
		{Type: protocol.AnyTLS, User: "alice", Port: "auto", Domain: "example.com", ShadowTLS: true},
		{Type: protocol.TUIC, User: "alice", Port: "auto", Domain: "example.com", Congestion: "wrong"},
	} {
		if _, err := a.ProtocolInstall(context.Background(), p); err == nil {
			t.Fatalf("accepted invalid options: %#v", p)
		}
	}
	if _, err := os.Stat(a.LockDir); !os.IsNotExist(err) {
		t.Fatal("invalid arguments created operation state")
	}
}

func TestAutoPortsAvoidConfiguredPorts(t *testing.T) {
	used := map[int]bool{443: true, 8388: true, 8443: true, 9443: true}
	port, err := availableProtocolPort(protocol.AnyTLS, 0, used)
	if err != nil || used[port] || port == 0 {
		t.Fatalf("selected port %d: %v", port, err)
	}
	if _, err := availableProtocolPort(protocol.AnyTLS, 443, used); err == nil {
		t.Fatal("explicit conflicting port accepted")
	}
}

func TestSnellMultiListenPort(t *testing.T) {
	for _, listen := range []string{"0.0.0.0:24444,[::]:24444", "[::]:24444", "0.0.0.0:24444"} {
		if got := (&store.SnellConfig{Listen: listen}).Port(); got != 24444 {
			t.Fatalf("%s: %d", listen, got)
		}
	}
}

func TestBulkEnrollmentPreservesExistingCredentialsAndSnellOwner(t *testing.T) {
	a := protocolTestApp(t)
	snapshot, err := a.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := snapshot.Store
	for i, pt := range []protocol.Type{protocol.VLESS, protocol.VLESSReality, protocol.TUIC, protocol.AnyTLS, protocol.Snell} {
		if _, err := protocol.Install(s, protocol.InstallParams{ProtoType: pt, Port: 23000 + i, UserName: "alice", Domain: "example.com", SNI: "www.netbsd.org"}); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := user.Add(s, "bob", true)
	if err != nil || !changed {
		t.Fatalf("bulk enrollment: %v %v", changed, err)
	}
	before := derived.Membership(s)["bob"]
	// Every node but snell, which has a single owner.
	if len(before) != 4 {
		t.Fatalf("bob has %d memberships", len(before))
	}
	changed, err = user.Add(s, "bob", true)
	if err != nil || changed {
		t.Fatalf("repeat enrollment: %v %v", changed, err)
	}
	after := derived.Membership(s)["bob"]
	for i, membership := range before {
		if membership.UserID != after[i].UserID {
			t.Fatal("repeat enrollment rotated a credential")
		}
	}
	if owner := s.UserMeta.Name[store.UserKey("snell", store.SnellTag, s.SnellConf.PSK)]; owner != "alice" {
		t.Fatalf("snell owner changed to %q", owner)
	}
}

// Removal takes the row number the guidance printed, because that list and the
// command are read in one breath. A tag still works: that is what a script
// holds, and what --json reports.
func TestProtocolRemoveAcceptsARowNumberOrATag(t *testing.T) {
	nodes := []ProtocolNode{
		{Tag: "anytls_443", Type: "anytls", Port: 443},
		{Tag: "vless_2083", Type: "vless", Port: 2083},
		{Tag: "vless_reality_2053", Type: "vless", Port: 2053},
	}
	for selector, want := range map[string]string{
		"1":                  "anytls_443",
		"3":                  "vless_reality_2053",
		"vless_2083":         "vless_2083",
		"vless_reality_2053": "vless_reality_2053",
	} {
		got, err := selectNode(nodes, selector)
		if err != nil || got == nil {
			t.Fatalf("selectNode(%q) = %v, %v", selector, got, err)
		}
		if got.Tag != want {
			t.Fatalf("selectNode(%q) chose %q, want %q", selector, got.Tag, want)
		}
	}
	// A number outside the list is a mistake worth naming, not a silent no-op.
	for _, outside := range []string{"0", "4", "-1"} {
		if _, err := selectNode(nodes, outside); err == nil {
			t.Fatalf("selectNode(%q) accepted a row that does not exist", outside)
		}
	}
	// An unknown tag is still "nothing to remove", which the caller reports as
	// an unchanged result rather than an error.
	if got, err := selectNode(nodes, "nosuchtag"); got != nil || err != nil {
		t.Fatalf("unknown tag gave %v, %v", got, err)
	}
	// A tag that looks like a number is a tag first.
	numeric := []ProtocolNode{{Tag: "2", Type: "vless", Port: 443}, {Tag: "other", Type: "tuic", Port: 444}}
	if got, _ := selectNode(numeric, "2"); got == nil || got.Tag != "2" {
		t.Fatalf("a tag spelled like a row number lost to the row: %v", got)
	}
}

// Removal is idempotent: a tag that names nothing removed nothing, and that is
// exit 0 with changed false. What the payload has to say is which of the two
// happened, because "tag": "vless" on its own reads like a node that went away.
func TestRemovalPayloadsSayWhetherAnythingWasRemoved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	a := protocolTestApp(t)
	// Removal commits, and committing a node configuration validates it with
	// the core. A stub accepts it: what this test is about is the payload.
	saved := config.SingBoxBin
	config.SingBoxBin = filepath.Join(dir, "sing-box")
	t.Cleanup(func() { config.SingBoxBin = saved })
	if err := os.WriteFile(config.SingBoxBin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Install(snapshot.Store, protocol.InstallParams{ProtoType: protocol.VLESSReality, Port: 24443, UserName: "alice", SNI: "www.netbsd.org"}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}

	// The type name the listing shows in its first column is not a tag. Typing
	// it is the likely mistake, so the answer has to be legible.
	missed, err := a.ProtocolRemove(ctx, "vless", "")
	if err != nil {
		t.Fatalf("unknown tag: %v", err)
	}
	fields, _ := missed.Data.(map[string]any)
	if missed.Changed || fields["removed"] != false || fields["tag"] != "vless" {
		t.Fatalf("unknown tag reported %#v changed=%t", missed.Data, missed.Changed)
	}

	removed, err := a.ProtocolRemove(ctx, "vless_reality_24443", "")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ = removed.Data.(map[string]any)
	if !removed.Changed || fields["removed"] != true || fields["tag"] != "vless_reality_24443" {
		t.Fatalf("removal reported %#v changed=%t", removed.Data, removed.Changed)
	}

	// A user deletion answers about the user. It used to answer with the node
	// inventory, because it shares the commit path with node removal.
	if _, err := a.UserAdd(ctx, "bob", false); err != nil {
		t.Fatal(err)
	}
	deleted, err := a.UserDelete(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ = deleted.Data.(map[string]any)
	if !deleted.Changed || fields["user"] != "bob" || fields["removed"] != true {
		t.Fatalf("user deletion reported %#v changed=%t", deleted.Data, deleted.Changed)
	}
	if _, present := fields["nodes"]; present {
		t.Fatalf("user deletion answered with the node inventory: %#v", deleted.Data)
	}
}

// A node that exists was issued for a domain, and that domain is on disk.
// Requiring --domain again to enrol a second user into it asked a question the
// answer to was already stored, and getting it wrong was a conflict error.
func TestJoiningAnExistingNodeInheritsItsDomain(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a := protocolTestApp(t)
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Install(snapshot.Store, protocol.InstallParams{ProtoType: protocol.AnyTLS, Port: 2083, UserName: "alice", Domain: "node.example"}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}

	// Creating a node still requires one, and the message says why it was not
	// needed a moment ago.
	_, err = a.ProtocolInstall(ctx, ProtocolOptions{Type: protocol.AnyTLS, User: "bob", Port: "9443"})
	var detail *Error
	if !errors.As(err, &detail) || detail.Code != "invalid_argument" || !strings.Contains(detail.Message, "--domain is required") {
		t.Fatalf("creating a node without a domain gave %v", err)
	}

	// Joining the existing one does not. The install itself needs the core and
	// systemd, so this asserts how far it gets: past the domain check, into the
	// work that a test host cannot do.
	_, err = a.ProtocolInstall(ctx, ProtocolOptions{Type: protocol.AnyTLS, User: "bob", Port: "2083"})
	if errors.As(err, &detail) && strings.Contains(detail.Message, "--domain") {
		t.Fatalf("joining an existing node still demanded a domain: %v", err)
	}

	// A domain that disagrees with the node is still refused rather than
	// silently ignored.
	_, err = a.ProtocolInstall(ctx, ProtocolOptions{Type: protocol.AnyTLS, User: "bob", Port: "2083", Domain: "other.example"})
	if !errors.As(err, &detail) || !strings.Contains(detail.Message, "conflicts") {
		t.Fatalf("a conflicting domain gave %v", err)
	}
}

// Uninstall owns the lock directory as much as the runtime root: both are
// created by this program and neither is shared. Left behind, /run/lock survived
// every reinstall on a host that had not rebooted.
func TestUninstallPreviewClaimsTheLockDirectory(t *testing.T) {
	a := protocolTestApp(t)
	scope, err := a.uninstallScope()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(scope, a.LockDir) {
		t.Fatalf("the removal scope does not claim %q: %#v", a.LockDir, scope)
	}
	if !slices.Contains(scope, config.WorkDir) {
		t.Fatalf("the removal scope does not claim the runtime root: %#v", scope)
	}
	// The preview lists only what exists: the lock directory this fixture
	// created, not the runtime root it does not have.
	result, err := a.Uninstall(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := result.Data.(map[string]any)["paths"].([]string)
	if !slices.Contains(paths, a.LockDir) {
		t.Fatalf("the preview left out the existing lock directory: %#v", paths)
	}
	for _, path := range paths {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("the preview lists %q, which does not exist", path)
		}
	}
	if result.Changed {
		t.Fatal("a preview reported a change")
	}
}

// The removal guidance prints a name in its first column and then takes what
// was typed. Printing `tuic` and refusing `tuic` told the reader something
// untrue, so the printed name selects the node it names.
func TestNodeSelectorAcceptsTheNameTheGuidancePrints(t *testing.T) {
	nodes := []ProtocolNode{
		{Tag: "anytls_2083", Type: "anytls", Port: 2083},
		{Tag: "snell-v6", Type: "snell", Port: 1443},
		{Tag: "tuic_26680", Type: "tuic", Port: 26680},
		{Tag: "vless_443", Type: "vless", Port: 443},
		{Tag: "vless_reality_2053", Type: "vless", Port: 2053},
	}
	// What the guidance prints for each row is what selects it.
	for _, node := range nodes {
		got, err := selectNode(nodes, NodeName(node))
		if err != nil || got == nil || got.Tag != node.Tag {
			t.Fatalf("the printed name %q selected %v, %v", NodeName(node), got, err)
		}
	}
	// Tags and row numbers still work, and a name that is on two ports is
	// ambiguous rather than a guess.
	if got, err := selectNode(nodes, "vless_443"); err != nil || got == nil || got.Tag != "vless_443" {
		t.Fatalf("full tag selected %v, %v", got, err)
	}
	if got, err := selectNode(nodes, "3"); err != nil || got == nil || got.Tag != "tuic_26680" {
		t.Fatalf("row number selected %v, %v", got, err)
	}
	twoPorts := []ProtocolNode{
		{Tag: "tuic_26680", Type: "tuic", Port: 26680},
		{Tag: "tuic_28168", Type: "tuic", Port: 28168},
	}
	_, err := selectNode(twoPorts, "tuic")
	var detail *Error
	if !errors.As(err, &detail) || !strings.Contains(detail.Message, "26680") || !strings.Contains(detail.Message, "28168") {
		t.Fatalf("an ambiguous name gave %v", err)
	}
	// A name that is nobody's is still nothing to remove rather than an error.
	if got, err := selectNode(nodes, "nosuchprotocol"); got != nil || err != nil {
		t.Fatalf("unknown selector gave %v, %v", got, err)
	}
}

// Deleting a user takes its exclusive nodes and its rules with it. The result
// says how many, because "user removed" alone left that to be discovered.
//
// The store is built directly rather than through the routing commands: those
// activate services, and what is under test here is the counting.
func TestUserRemovalReportsWhatItCascadedInto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	a := protocolTestApp(t)
	// Committing a node configuration validates it with the core; a stub
	// accepts it, since what is under test is the counting.
	saved := config.SingBoxBin
	config.SingBoxBin = filepath.Join(dir, "sing-box")
	t.Cleanup(func() { config.SingBoxBin = saved })
	if err := os.WriteFile(config.SingBoxBin, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Install(snapshot.Store, protocol.InstallParams{ProtoType: protocol.VLESSReality, Port: 2053, UserName: "alice", SNI: "www.netbsd.org"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"openai", "github"} {
		preset, ok := routing.FindPreset(name)
		if !ok {
			t.Fatalf("preset %q is gone", name)
		}
		if err := routing.SetRule(snapshot.Store, "alice", routing.PresetToRule(preset, "alice", "direct")); err != nil {
			t.Fatal(err)
		}
	}
	if err := snapshot.Store.Save(); err != nil {
		t.Fatal(err)
	}

	result, err := a.UserDelete(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := result.Data.(map[string]any)
	if fields["nodes_removed"] != 1 {
		t.Fatalf("exclusive nodes removed reported as %v, want 1: %#v", fields["nodes_removed"], fields)
	}
	if fields["rules_removed"] != 2 {
		t.Fatalf("rules removed reported as %v, want 2: %#v", fields["rules_removed"], fields)
	}
	after, err := a.ProtocolList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if remaining, _ := after.Data.(map[string]any)["nodes"].([]ProtocolNode); len(remaining) != 0 {
		t.Fatalf("the user's exclusive node survived: %#v", remaining)
	}

	// A user with nothing reports nothing rather than two zeroes.
	if _, err := a.UserAdd(ctx, "bob", false); err != nil {
		t.Fatal(err)
	}
	bare, err := a.UserDelete(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ = bare.Data.(map[string]any)
	if fields["nodes_removed"] != 0 || fields["rules_removed"] != 0 {
		t.Fatalf("a user with nothing cascaded into %#v", fields)
	}
}
