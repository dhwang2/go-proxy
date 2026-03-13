package subscription

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dhwang2/go-proxy/internal/store"
)

func TestRenderAllFormats(t *testing.T) {
	st := testSubscriptionStore(t)

	result, err := Render(st, RenderOptions{Host: "203.0.113.10", Format: FormatAll})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if result.Target.Preferred.Family != "ipv4" || result.Target.Preferred.Host != "203.0.113.10" {
		t.Fatalf("unexpected preferred target: %#v", result.Target.Preferred)
	}
	if len(result.Users) != 2 || result.Users[0] != "alice" || result.Users[1] != "bob" {
		t.Fatalf("unexpected users: %#v", result.Users)
	}

	alice := result.ByUser["alice"]
	if !containsPrefix(alice.Singbox, "vless://") {
		t.Fatalf("expected alice singbox vless link")
	}
	if !containsPrefix(alice.Singbox, "tuic://") {
		t.Fatalf("expected alice singbox tuic link")
	}
	if !containsPrefix(alice.Singbox, "anytls://") {
		t.Fatalf("expected alice singbox anytls link")
	}
	if !containsSubstring(alice.Surge, "tuic-v5") {
		t.Fatalf("expected alice surge tuic line")
	}

	bob := result.ByUser["bob"]
	if !containsPrefix(bob.Singbox, "trojan://") {
		t.Fatalf("expected bob singbox trojan link")
	}
	if !containsPrefix(bob.Singbox, "ss://") {
		t.Fatalf("expected bob singbox ss link")
	}
	if !containsSubstring(bob.Surge, "snell") {
		t.Fatalf("expected bob surge snell line")
	}
}

func TestRenderUserFilterAndFormat(t *testing.T) {
	st := testSubscriptionStore(t)

	result, err := Render(st, RenderOptions{Host: "subs.example.com", User: "Alice", Format: FormatSingbox})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if len(result.Users) != 1 || result.Users[0] != "alice" {
		t.Fatalf("unexpected filtered users: %#v", result.Users)
	}
	if len(result.ByUser["alice"].Singbox) == 0 {
		t.Fatalf("expected singbox links for alice")
	}
	if len(result.ByUser["alice"].Surge) != 0 {
		t.Fatalf("expected no surge links for singbox-only output")
	}
}

func TestDetectTargetFromInboundSNI(t *testing.T) {
	st := &store.Store{
		Config: &store.SingboxConfig{
			Inbounds: []store.Inbound{
				{
					Type:       "vless",
					Tag:        "vless-main",
					ListenPort: 443,
					Users:      []store.User{{ID: "u1"}},
					Raw:        rawMap(t, `{"tls":{"server_name":"edge.example.com"}}`),
				},
			},
		},
	}

	target, err := DetectTarget(st, "")
	if err != nil {
		t.Fatalf("DetectTarget returned error: %v", err)
	}
	if target.Preferred.Family != "domain" || target.Preferred.Host != "edge.example.com" {
		t.Fatalf("unexpected target: %#v", target.Preferred)
	}
}

func testSubscriptionStore(t *testing.T) *store.Store {
	t.Helper()
	meta := store.DefaultUserMeta()
	meta.Name["vless|vless-main|11111111-1111-1111-1111-111111111111"] = "alice"
	meta.Name["tuic|tuic-main|22222222-2222-2222-2222-222222222222"] = "alice"
	meta.Name["anytls|anytls-main|anytls-pass"] = "alice"
	meta.Name["trojan|trojan-main|trojan-pass"] = "bob"
	meta.Name["ss|ss-main|ss-pass"] = "bob"
	meta.Name["snell|snell-v5|snell-psk"] = "bob"
	meta.Groups["alice"] = store.Group{}
	meta.Groups["bob"] = store.Group{}

	return &store.Store{
		Config: &store.SingboxConfig{
			Inbounds: []store.Inbound{
				{
					Type:       "vless",
					Tag:        "vless-main",
					ListenPort: 443,
					Users: []store.User{{
						ID:   "11111111-1111-1111-1111-111111111111",
						Name: "alice",
						Raw:  rawMap(t, `{"flow":"xtls-rprx-vision"}`),
					}},
					Raw: rawMap(t, `{"tls":{"server_name":"edge.example.com","reality":{"private_key":"PkQbdsqhXNdyFSewdqK1hjxfVWxBxgP75ts7FCSrUN8","short_id":["abcd1234"]}}}`),
				},
				{
					Type:       "tuic",
					Tag:        "tuic-main",
					ListenPort: 8443,
					Users: []store.User{{
						ID:       "22222222-2222-2222-2222-222222222222",
						Password: "tuic-pass",
					}},
					Raw: rawMap(t, `{"tls":{"server_name":"edge.example.com"}}`),
				},
				{
					Type:       "trojan",
					Tag:        "trojan-main",
					ListenPort: 443,
					Users: []store.User{{
						Password: "trojan-pass",
					}},
					Raw: rawMap(t, `{"tls":{"server_name":"edge.example.com","alpn":["h2","http/1.1"]}}`),
				},
				{
					Type:       "anytls",
					Tag:        "anytls-main",
					ListenPort: 10443,
					Users: []store.User{{
						Password: "anytls-pass",
					}},
					Raw: rawMap(t, `{"tls":{"server_name":"edge.example.com"}}`),
				},
				{
					Type:       "shadowsocks",
					Tag:        "ss-main",
					ListenPort: 8388,
					Users: []store.User{{
						Password: "ss-pass",
						Method:   "2022-blake3-aes-128-gcm",
					}},
				},
			},
		},
		UserMeta: meta,
		SnellConf: &store.SnellConfig{Values: map[string]string{
			"listen": "0.0.0.0:8444",
			"psk":    "snell-psk",
		}},
	}
}

func rawMap(t *testing.T, payload string) map[string]json.RawMessage {
	t.Helper()
	if strings.TrimSpace(payload) == "" {
		return nil
	}
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("invalid raw payload: %v", err)
	}
	return out
}

func containsPrefix(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func containsSubstring(lines []string, sub string) bool {
	for _, line := range lines {
		if strings.Contains(line, sub) {
			return true
		}
	}
	return false
}
