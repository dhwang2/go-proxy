package subscription

import (
	"strings"
	"testing"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

func TestRenderSurgeTUICIncludesRequiredParams(t *testing.T) {
	ib := &store.Inbound{
		Type:       "tuic",
		Tag:        "tuic_443",
		ListenPort: 443,
		Users: []store.User{
			{Name: "alice", UUID: "11111111-1111-1111-1111-111111111111", Password: "pw"},
		},
		TLS: &store.TLSConfig{ServerName: "example.com"},
	}
	entry := derived.MembershipEntry{
		Tag:      ib.Tag,
		Port:     ib.ListenPort,
		UserID:   "11111111-1111-1111-1111-111111111111",
		UserName: "alice",
	}

	got := renderSurge(ib, entry, "example.com")
	for _, want := range []string{
		"password=pw",
		"uuid=11111111-1111-1111-1111-111111111111",
		"skip-cert-verify=false",
		"congestion-controller=bbr",
		"udp-relay=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSurge(tuic) missing %q in %q", want, got)
		}
	}
}

func TestRenderSnellSurgeIncludesRequiredParams(t *testing.T) {
	entry := derived.MembershipEntry{
		Proto:    store.SnellTag,
		Tag:      store.SnellTag,
		Port:     8443,
		UserID:   "secret",
		UserName: "alice",
	}
	conf := &store.SnellConfig{Listen: "0.0.0.0:8443", PSK: "secret"}

	got := renderSnellSurge(entry, conf, "1.2.3.4")
	for _, want := range []string{
		"psk=secret",
		"version=5",
		"reuse=true",
		"tfo=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSnellSurge() missing %q in %q", want, got)
		}
	}
}

func TestRenderInfersLegacySnellOwnerFromSingleActiveInboundUser(t *testing.T) {
	s := &store.Store{
		SingBox: &store.SingBoxConfig{
			Inbounds: []store.Inbound{
				{
					Type:       "trojan",
					Tag:        "trojan_443",
					ListenPort: 443,
					Users: []store.User{
						{Name: "u1", Password: "secret"},
					},
				},
			},
		},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
		SnellConf:    &store.SnellConfig{Listen: "0.0.0.0:8448", PSK: "legacy-psk"},
	}
	s.UserMeta.Groups["~/.groups"] = []string{"u1", "u2"}

	links := Render(s, "u1", FormatSurge, "example.com")
	if len(links) != 2 {
		t.Fatalf("links len = %d, want 2", len(links))
	}
	if got := links[1].Content; !strings.Contains(got, "psk=legacy-psk") {
		t.Fatalf("legacy snell link not rendered correctly: %q", got)
	}
}
