package subscription

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"go-proxy/internal/crypto"
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

func TestRealitySubscriptionsIncludeClientKey(t *testing.T) {
	kp, err := crypto.GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	ib := &store.Inbound{
		Type: "vless", Tag: "vless_reality_24443", ListenPort: 24443,
		Users: []store.User{{Name: "alice", UUID: "test-uuid", Flow: "xtls-rprx-vision"}},
		TLS: &store.TLSConfig{Enabled: true, ServerName: "www.microsoft.com", Reality: &store.RealityConfig{
			Enabled: true, PrivateKey: kp.PrivateKey, ShortID: []string{"01234567"},
		}},
	}
	entry := derived.MembershipEntry{Tag: ib.Tag, UserName: "alice", UserID: "test-uuid"}
	u := &ib.Users[0]
	tls, err := buildClientTLS(ib.TLS)
	if err != nil {
		t.Fatal(err)
	}
	content := renderMihomo(ib, entry, "192.0.2.1", "name", u, tls)
	for _, want := range []string{
		`reality-opts: {public-key: "` + kp.PublicKey + `",short-id: "01234567"}`,
		`servername: "www.microsoft.com"`,
		`flow: "xtls-rprx-vision"`,
		`client-fingerprint: "chrome"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("incomplete Reality client config, missing %s:\n%s", want, content)
		}
	}
	uri := renderURI(ib, entry, "192.0.2.1", "name", u, tls)
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"pbk": kp.PublicKey, "sid": "01234567", "sni": "www.microsoft.com", "security": "reality", "fp": "chrome", "flow": "xtls-rprx-vision"} {
		if parsed.Query().Get(key) != want {
			t.Errorf("URI %s missing or incorrect", key)
		}
	}
	if strings.Contains(content, kp.PrivateKey) || strings.Contains(uri, kp.PrivateKey) {
		t.Fatal("subscription leaked the Reality private key")
	}
	ib.TLS.Reality.PrivateKey = "invalid"
	if _, err := buildClientTLS(ib.TLS); err == nil {
		t.Fatal("invalid Reality key must not produce an unusable subscription")
	}
}

// Exercises Reality through Renderer.Render rather than the render helpers
// directly, covering the lazily built per-node client TLS cache.
func TestRealityRendersThroughRendererForBothFormats(t *testing.T) {
	kp, err := crypto.GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	ib := store.Inbound{
		Type: "vless", Tag: "vless_reality_24443", ListenPort: 24443,
		Users: []store.User{{Name: "alice", UUID: "test-uuid", Flow: "xtls-rprx-vision"}},
		TLS: &store.TLSConfig{Enabled: true, ServerName: "www.kernel.org", Reality: &store.RealityConfig{
			Enabled: true, PrivateKey: kp.PrivateKey, ShortID: []string{"01234567"},
		}},
	}
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{ib}}, UserMeta: store.NewUserManagement()}
	entry := derived.Membership(s)["alice"][0]

	// FormatURI first: it is the only path that dereferences the cached TLS
	// block, and it must build the cache itself rather than rely on a prior
	// sing-box render having populated it.
	renderer := NewRenderer(s, nil, "192.0.2.1", []SurgeTarget{{Host: "192.0.2.1", Family: "v4"}})
	for _, format := range []Format{FormatURI, FormatMihomo} {
		links, err := renderer.Render(context.Background(), entry, format)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if len(links) != 1 {
			t.Fatalf("%s produced %d links", format, len(links))
		}
		if !strings.Contains(links[0].Content, kp.PublicKey) {
			t.Fatalf("%s lost the Reality public key: %s", format, links[0].Content)
		}
		if strings.Contains(links[0].Content, kp.PrivateKey) {
			t.Fatalf("%s leaked the Reality private key", format)
		}
	}

	// A node whose Reality private key cannot be parsed must fail the export
	// instead of silently emitting an unusable link.
	bad := s.SingBox.Inbounds[0]
	bad.TLS = &store.TLSConfig{Enabled: true, ServerName: "www.kernel.org", Reality: &store.RealityConfig{
		Enabled: true, PrivateKey: "invalid", ShortID: []string{"01234567"},
	}}
	broken := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{bad}}, UserMeta: store.NewUserManagement()}
	brokenRenderer := NewRenderer(broken, nil, "192.0.2.1", []SurgeTarget{{Host: "192.0.2.1", Family: "v4"}})
	brokenEntry := derived.Membership(broken)["alice"][0]
	for _, format := range []Format{FormatURI, FormatMihomo} {
		links, err := brokenRenderer.Render(context.Background(), brokenEntry, format)
		if err == nil {
			t.Fatalf("%s exported a node with an unusable Reality key: %v", format, links)
		}
		if strings.Contains(err.Error(), "invalid") && strings.Contains(err.Error(), kp.PrivateKey) {
			t.Fatalf("%s error leaked key material", format)
		}
	}
}
