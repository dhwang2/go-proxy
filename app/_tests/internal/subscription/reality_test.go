package subscription

import (
	"encoding/json"
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
	content := renderSingBox(ib, entry, "192.0.2.1")
	var out struct {
		Flow string `json:"flow"`
		TLS  struct {
			ServerName string `json:"server_name"`
			Reality    struct {
				PublicKey string `json:"public_key"`
				ShortID   string `json:"short_id"`
			} `json:"reality"`
			UTLS struct {
				Enabled     bool   `json:"enabled"`
				Fingerprint string `json:"fingerprint"`
			} `json:"utls"`
		} `json:"tls"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		t.Fatal(err)
	}
	if out.TLS.Reality.PublicKey != kp.PublicKey || out.TLS.Reality.ShortID != "01234567" ||
		out.TLS.ServerName != "www.microsoft.com" || out.Flow != "xtls-rprx-vision" ||
		!out.TLS.UTLS.Enabled || out.TLS.UTLS.Fingerprint != "chrome" {
		t.Fatalf("incomplete Reality client config: %s", content)
	}
	uri := renderURI(ib, entry, "192.0.2.1")
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
	if renderSingBox(ib, entry, "192.0.2.1") != "" || renderURI(ib, entry, "192.0.2.1") != "" {
		t.Fatal("invalid Reality key must not produce an unusable subscription")
	}
}
