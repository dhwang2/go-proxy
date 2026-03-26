package subscription

import (
	"strings"
	"testing"

	"go-proxy/internal/derived"
	"go-proxy/internal/service"
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

func TestRenderShadowTLSShadowsocksSurgeIncludesShadowTLSParams(t *testing.T) {
	ib := &store.Inbound{
		Type:       "shadowsocks",
		Tag:        "shadowsocks_443",
		ListenPort: 443,
		Method:     "2022-blake3-aes-128-gcm",
		Password:   "server-key",
	}
	entry := derived.MembershipEntry{
		Proto:    "shadowsocks",
		Tag:      ib.Tag,
		Port:     ib.ListenPort,
		UserID:   "user-key",
		UserName: "alice",
	}
	binding := service.ShadowTLSBinding{
		ListenPort:   8443,
		BackendPort:  443,
		BackendProto: "ss",
		SNI:          "www.microsoft.com",
		Password:     "shadow-pass",
		Version:      3,
	}

	got := renderShadowTLSShadowsocksSurge(ib, entry, binding, "example.com")
	for _, want := range []string{
		"8443",
		"shadow-tls-password=shadow-pass",
		"shadow-tls-sni=www.microsoft.com",
		"shadow-tls-version=3",
		"udp-relay=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderShadowTLSShadowsocksSurge() missing %q in %q", want, got)
		}
	}
}

func TestRenderShadowTLSSnellSurgeIncludesShadowTLSParams(t *testing.T) {
	entry := derived.MembershipEntry{
		Proto:    store.SnellTag,
		Tag:      store.SnellTag,
		Port:     1443,
		UserID:   "secret",
		UserName: "alice",
	}
	conf := &store.SnellConfig{Listen: "0.0.0.0:1443", PSK: "secret"}
	binding := service.ShadowTLSBinding{
		ListenPort:   8443,
		BackendPort:  1443,
		BackendProto: "snell",
		SNI:          "www.microsoft.com",
		Password:     "shadow-pass",
		Version:      3,
	}

	got := renderShadowTLSSnellSurge(entry, conf, binding, "1.2.3.4")
	for _, want := range []string{
		"8443",
		"psk=secret",
		"shadow-tls-password=shadow-pass",
		"shadow-tls-sni=www.microsoft.com",
		"shadow-tls-version=3",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderShadowTLSSnellSurge() missing %q in %q", want, got)
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

func TestRenderUsesShadowTLSFrontForShadowsocksSurgeAndKeepsURI(t *testing.T) {
	prev := listShadowTLSBindings
	listShadowTLSBindings = func(*store.Store) ([]service.ShadowTLSBinding, error) {
		return []service.ShadowTLSBinding{
			{
				ListenPort:   8443,
				BackendPort:  443,
				BackendProto: "ss",
				SNI:          "www.microsoft.com",
				Password:     "shadow-pass",
				Version:      3,
			},
		}, nil
	}
	defer func() { listShadowTLSBindings = prev }()

	s := &store.Store{
		SingBox: &store.SingBoxConfig{
			Inbounds: []store.Inbound{
				{
					Type:       "shadowsocks",
					Tag:        "shadowsocks_443",
					ListenPort: 443,
					Method:     "2022-blake3-aes-128-gcm",
					Password:   "server-key",
					Users: []store.User{
						{Name: "alice", Password: "user-key"},
					},
				},
			},
		},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}

	surgeLinks := Render(s, "alice", FormatSurge, "example.com")
	if len(surgeLinks) != 1 {
		t.Fatalf("surge links len = %d, want 1", len(surgeLinks))
	}
	if surgeLinks[0].Port != 8443 {
		t.Fatalf("surge link port = %d, want 8443", surgeLinks[0].Port)
	}
	for _, want := range []string{"8443", "shadow-tls-password=shadow-pass", "shadow-tls-sni=www.microsoft.com"} {
		if !strings.Contains(surgeLinks[0].Content, want) {
			t.Fatalf("surge link missing %q in %q", want, surgeLinks[0].Content)
		}
	}

	uriLinks := Render(s, "alice", FormatURI, "example.com")
	if len(uriLinks) != 1 {
		t.Fatalf("uri links len = %d, want 1", len(uriLinks))
	}
	if !strings.Contains(uriLinks[0].Content, "ss://") {
		t.Fatalf("uri link missing ss scheme: %q", uriLinks[0].Content)
	}
	if !strings.Contains(uriLinks[0].Content, "@example.com:443") {
		t.Fatalf("uri link host/port mismatch: %q", uriLinks[0].Content)
	}
}
