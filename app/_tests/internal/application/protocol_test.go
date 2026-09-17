package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/protocol"
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
	result, err := a.Subscription(ctx, SubscriptionOptions{User: "alice", Target: "192.0.2.1", Format: subscription.FormatSingBox})
	if err == nil || len(result.Raw) > 0 {
		t.Fatalf("unsupported mixed export must fail before output: %#v %v", result, err)
	}
	result, err = a.Subscription(ctx, SubscriptionOptions{User: "alice", Node: "vless_reality_24443", Target: "192.0.2.1", Format: subscription.FormatSingBox})
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err = json.Unmarshal(result.Raw, &native); err != nil || len(native.Outbounds) != 1 {
		t.Fatalf("native export: %s %v", result.Raw, err)
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
	if !strings.Contains(string(encoded), `"sing-box"`) || !strings.Contains(string(encoded), `"snell-v6"`) {
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
		{Type: protocol.Shadowsocks, User: "alice", Port: "70000", Method: crypto.DefaultSSMethod},
		{Type: protocol.VLESS, User: "alice", Port: "auto"},
		{Type: protocol.VLESSReality, User: "alice", Port: "auto", Domain: "example.com"},
		{Type: protocol.VLESSReality, User: "alice", Port: "auto", SNI: "www.apple.com"},
		{Type: protocol.Shadowsocks, User: "alice", Port: "auto", Method: crypto.DefaultSSMethod, ShadowTLS: true},
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
	port, err := availableProtocolPort(protocol.Shadowsocks, 0, used)
	if err != nil || used[port] || port == 0 {
		t.Fatalf("selected port %d: %v", port, err)
	}
	if _, err := availableProtocolPort(protocol.Shadowsocks, 443, used); err == nil {
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
	for i, pt := range []protocol.Type{protocol.Shadowsocks, protocol.VLESS, protocol.VLESSReality, protocol.TUIC, protocol.AnyTLS, protocol.Snell} {
		if _, err := protocol.Install(s, protocol.InstallParams{ProtoType: pt, Port: 23000 + i, UserName: "alice", Domain: "example.com", SNI: "www.netbsd.org"}); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := user.Add(s, "bob", true)
	if err != nil || !changed {
		t.Fatalf("bulk enrollment: %v %v", changed, err)
	}
	before := derived.Membership(s)["bob"]
	if len(before) != 5 {
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
