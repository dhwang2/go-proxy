package subscription

import (
	"context"
	"os"
	"strings"
	"testing"

	"go-proxy/internal/derived"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

func TestMain(m *testing.M) {
	// Stub serverName so hostname-prefixed surge tags are deterministic.
	prev := serverName
	serverName = func() string { return "" }
	code := m.Run()
	serverName = prev
	os.Exit(code)
}

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

	got := renderSurge(ib, entry, "1.2.3.4", "example.com", "", &ib.Users[0])
	if !strings.HasPrefix(got, "tuic-alice = tuic-v5") {
		t.Fatalf("renderSurge(tuic) tag = %q, want prefix %q", got, "tuic-alice = tuic-v5")
	}
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

	got := renderSnellSurge(entry, conf, "1.2.3.4", "")
	if !strings.HasPrefix(got, "snell-alice = snell") {
		t.Fatalf("renderSnellSurge() tag = %q, want prefix %q", got, "snell-alice = snell")
	}
	for _, want := range []string{
		"psk=secret",
		"version=6",
		"reuse=true",
		"tfo=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSnellSurge() missing %q in %q", want, got)
		}
	}
}

func TestSurgeIPv6HostsAreUnbracketed(t *testing.T) {
	entry := derived.MembershipEntry{UserName: "alice", UserID: "secret"}
	ib := &store.Inbound{Type: "shadowsocks", ListenPort: 1443, Method: "aes-128-gcm"}
	conf := &store.SnellConfig{Listen: "0.0.0.0:1443", PSK: "secret"}
	binding := service.ShadowTLSBinding{ListenPort: 8443, Password: "shadow-pass", SNI: "example.com", Version: 3}
	host := "2001:db8::1"
	for name, got := range map[string]string{
		"sing-box":         renderSurge(ib, entry, host, "example.com", "", nil),
		"snell":            renderSnellSurge(entry, conf, host, ""),
		"shadow-tls-ss":    renderShadowTLSShadowsocksSurge(ib, entry, binding, host, ""),
		"shadow-tls-snell": renderShadowTLSSnellSurge(entry, conf, binding, host, ""),
	} {
		if !strings.Contains(got, ", "+host+", ") || strings.Contains(got, "["+host+"]") {
			t.Errorf("%s IPv6 host is not a bare address: %s", name, got)
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

	got := renderShadowTLSShadowsocksSurge(ib, entry, binding, "1.2.3.4", "")
	if !strings.HasPrefix(got, "ss-alice = ss") {
		t.Fatalf("renderShadowTLSShadowsocksSurge() tag = %q, want prefix %q", got, "ss-alice = ss")
	}
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

	got := renderShadowTLSSnellSurge(entry, conf, binding, "1.2.3.4", "")
	if !strings.HasPrefix(got, "snell-alice = snell") {
		t.Fatalf("renderShadowTLSSnellSurge() tag = %q, want prefix %q", got, "snell-alice = snell")
	}
	for _, want := range []string{
		"8443",
		"psk=secret",
		"version=6",
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
					Type:       "anytls",
					Tag:        "anytls_443",
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

	links := renderForUser(t, s, nil, "u1", FormatSurge, "1.2.3.4")
	if len(links) != 2 {
		t.Fatalf("links len = %d, want 2", len(links))
	}
	if got := links[1].Content; !strings.Contains(got, "psk=legacy-psk") {
		t.Fatalf("legacy snell link not rendered correctly: %q", got)
	}
}

func TestRenderUsesShadowTLSFrontAndRejectsUnsupportedURI(t *testing.T) {
	bindings := []service.ShadowTLSBinding{
		{
			ListenPort:   8443,
			BackendPort:  443,
			BackendProto: "ss",
			SNI:          "www.microsoft.com",
			Password:     "shadow-pass",
			Version:      3,
		},
	}

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

	surgeLinks := renderForUser(t, s, bindings, "alice", FormatSurge, "1.2.3.4")
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

	renderer := NewRenderer(s, bindings, "1.2.3.4", []SurgeTarget{{Family: "v4", Host: "1.2.3.4"}})
	for _, format := range []Format{FormatURI, FormatSingBox} {
		if _, err := renderer.Render(context.Background(), derived.Membership(s)["alice"][0], format); err == nil {
			t.Fatalf("%s export must not discard the wrapper", format)
		}
	}
}

func renderForUser(t *testing.T, s *store.Store, bindings []service.ShadowTLSBinding, name string, format Format, host string) []Link {
	t.Helper()
	renderer := NewRenderer(s, bindings, host, []SurgeTarget{{Host: host}})
	var links []Link
	for _, entry := range derived.Membership(s)[name] {
		generated, err := renderer.Render(context.Background(), entry, format)
		if err != nil {
			t.Fatal(err)
		}
		links = append(links, generated...)
	}
	return links
}
