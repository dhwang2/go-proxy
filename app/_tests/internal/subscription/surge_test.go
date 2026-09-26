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

	got := renderSurge(ib, entry, "1.2.3.4", "example.com", "tuic-alice", &ib.Users[0])
	if !strings.HasPrefix(got, "tuic-alice = tuic-v5") {
		t.Fatalf("renderSurge(tuic) tag = %q, want prefix %q", got, "tuic-alice = tuic-v5")
	}
	for _, want := range []string{
		"password=pw",
		"uuid=11111111-1111-1111-1111-111111111111",
		"skip-cert-verify=false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSurge(tuic) missing %q in %q", want, got)
		}
	}
	// Neither is a Surge tuic-v5 parameter.
	for _, unwanted := range []string{"congestion-controller", "udp-relay"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("renderSurge(tuic) carries %q, which Surge does not define: %q", unwanted, got)
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

	got := renderSnellSurge(entry, conf, "1.2.3.4", "snell-alice")
	if !strings.HasPrefix(got, "snell-alice = snell") {
		t.Fatalf("renderSnellSurge() tag = %q, want prefix %q", got, "snell-alice = snell")
	}
	for _, want := range []string{
		"psk=secret",
		"version=6",
		"mode=default",
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
	conf := &store.SnellConfig{Listen: "0.0.0.0:1443", PSK: "secret"}
	binding := service.ShadowTLSBinding{ListenPort: 8443, Password: "shadow-pass", SNI: "example.com", Version: 3}
	host := "2001:db8::1"
	for name, got := range map[string]string{
		"snell":            renderSnellSurge(entry, conf, host, "snell-alice"),
		"shadow-tls-snell": renderShadowTLSSnellSurge(entry, conf, binding, host, "snell-alice"),
	} {
		if !strings.Contains(got, ", "+host+", ") || strings.Contains(got, "["+host+"]") {
			t.Errorf("%s IPv6 host is not a bare address: %s", name, got)
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

	got := renderShadowTLSSnellSurge(entry, conf, binding, "1.2.3.4", "snell-alice")
	if !strings.HasPrefix(got, "snell-alice = snell") {
		t.Fatalf("renderShadowTLSSnellSurge() tag = %q, want prefix %q", got, "snell-alice = snell")
	}
	for _, want := range []string{
		"8443",
		"psk=secret",
		"version=6",
		"mode=default",
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

func TestRenderUsesShadowTLSFrontAndRejectsUnsupportedFormats(t *testing.T) {
	bindings := []service.ShadowTLSBinding{
		{
			ListenPort:   8443,
			BackendPort:  443,
			BackendProto: "snell",
			SNI:          "www.microsoft.com",
			Password:     "shadow-pass",
			Version:      3,
		},
	}

	s := &store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
		SnellConf:    &store.SnellConfig{Listen: "0.0.0.0:443", PSK: "server-psk"},
	}
	s.UserMeta.Groups["~/.groups"] = []string{"alice"}

	surgeLinks := renderForUser(t, s, bindings, "alice", FormatSurge, "1.2.3.4")
	if len(surgeLinks) != 1 {
		t.Fatalf("surge links len = %d, want 1", len(surgeLinks))
	}
	// The link points at the wrapper, not at the backend it fronts.
	if surgeLinks[0].Port != 8443 {
		t.Fatalf("surge link port = %d, want 8443", surgeLinks[0].Port)
	}
	for _, want := range []string{"8443", "shadow-tls-password=shadow-pass", "shadow-tls-sni=www.microsoft.com"} {
		if !strings.Contains(surgeLinks[0].Content, want) {
			t.Fatalf("surge link missing %q in %q", want, surgeLinks[0].Content)
		}
	}

	// Neither of the other two can drive the wrapper, so neither may quietly
	// hand out a link to the backend port.
	renderer := NewRenderer(s, bindings, "1.2.3.4", []SurgeTarget{{Family: "v4", Host: "1.2.3.4"}})
	for _, format := range []Format{FormatURI, FormatMihomo} {
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

// A link name says the protocol once. It used to say it twice, because the
// node tag begins with its own protocol and the tag was appended to it:
// "gcp-oregon-snell-snell-v6-v4-alice".
func TestLinkNamesDoNotRepeatTheProtocol(t *testing.T) {
	s := &store.Store{
		SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{
			{Type: "anytls", Tag: "anytls_443", ListenPort: 443,
				Users: []store.User{{Name: "alice", Password: "pw"}},
				TLS:   &store.TLSConfig{Enabled: true, ServerName: "example.com"}},
		}},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
		SnellConf:    &store.SnellConfig{Listen: "0.0.0.0:1443", PSK: "secret"},
	}
	s.UserMeta.Groups["~/.groups"] = []string{"alice"}
	renderer := NewRenderer(s, nil, "1.2.3.4", []SurgeTarget{
		{Host: "1.2.3.4", Family: "v4"}, {Host: "2001:db8::1", Family: "v6"},
	})
	seen := map[string]bool{}
	for _, entry := range derived.Membership(s)["alice"] {
		for _, format := range renderer.Formats(entry) {
			links, err := renderer.Render(context.Background(), entry, format)
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				name := linkNameOf(t, link.Content)
				seen[name] = true
				// Each protocol word appears once in the name.
				for _, proto := range []string{"anytls", "snell"} {
					if strings.Count(name, proto) > 1 {
						t.Fatalf("%s names %q twice: %q", format, proto, name)
					}
				}
				// The node tag is not in the name. Snell's tag is not checked
				// here: "snell-v6" is also what snell over IPv6 is called,
				// since v6 is the address family, and the exact names are
				// asserted below.
				if strings.Contains(name, "anytls_443") {
					t.Fatalf("%s put the node tag in the name: %q", format, name)
				}
			}
		}
	}
	// Only one node per protocol per user here, so no port is needed.
	for name := range seen {
		if strings.Contains(name, "443") || strings.Contains(name, "1443") {
			t.Fatalf("a port was added where nothing was ambiguous: %q", name)
		}
	}
	for _, want := range []string{"snell-v4-alice", "snell-v6-alice", "anytls-v4-alice", "anytls-v6-alice"} {
		if !seen[want] {
			t.Fatalf("expected a link named %q, got %v", want, seen)
		}
	}
}

// linkNameOf pulls the name out of whichever shape the format uses.
func linkNameOf(t *testing.T, content string) string {
	t.Helper()
	switch {
	case strings.Contains(content, " = "):
		return strings.TrimSpace(strings.SplitN(content, " = ", 2)[0])
	case strings.Contains(content, "#"):
		return content[strings.LastIndex(content, "#")+1:]
	case strings.Contains(content, `name: "`):
		rest := content[strings.Index(content, `name: "`)+len(`name: "`):]
		return rest[:strings.Index(rest, `"`)]
	}
	t.Fatalf("cannot find a name in %q", content)
	return ""
}

// Compacting a flow mapping may only drop the space after a comma. Dropping
// the one after a colon still parses -- into a mapping whose keys are the
// whole `name:"value"` string and whose values are all null -- so the config
// would be silently wrong rather than rejected.
func TestMihomoMappingCompactsOnlyWhereItIsSafe(t *testing.T) {
	entry := derived.MembershipEntry{Tag: "anytls_443", Port: 443, UserID: "pw", UserName: "alice", Proto: "anytls"}
	ib := &store.Inbound{Type: "anytls", Tag: "anytls_443", ListenPort: 443,
		TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"}}
	got := renderMihomo(ib, entry, "1.2.3.4", "anytls-alice", nil, nil)

	if strings.Contains(got, ", ") {
		t.Fatalf("a space survived after a comma: %q", got)
	}
	// Every colon that separates a key from its value keeps its space.
	for _, key := range []string{"name", "type", "server", "port", "sni"} {
		if !strings.Contains(got, key+": ") {
			t.Fatalf("%q lost the space after its colon, which changes what it means: %q", key, got)
		}
	}
	if strings.Contains(got, ":\"") {
		t.Fatalf("a colon sits against a quoted value: %q", got)
	}
}

// Surge assumes mode=default, so a server set otherwise is reachable only
// when the line carries the server's mode.
func TestRenderSnellSurgeCarriesTheServersMode(t *testing.T) {
	entry := derived.MembershipEntry{Proto: store.SnellTag, Tag: store.SnellTag, Port: 1443, UserID: "secret", UserName: "alice"}
	conf := &store.SnellConfig{Listen: "0.0.0.0:1443", PSK: "secret", Mode: "unshaped"}
	binding := service.ShadowTLSBinding{ListenPort: 8443, BackendPort: 1443, BackendProto: "snell", SNI: "www.example.org", Password: "p", Version: 3}
	for name, line := range map[string]string{
		"snell":            renderSnellSurge(entry, conf, "1.2.3.4", "snell-alice"),
		"shadow-tls-snell": renderShadowTLSSnellSurge(entry, conf, binding, "1.2.3.4", "snell-alice"),
	} {
		if !strings.Contains(line, ", mode=unshaped,") {
			t.Fatalf("%s line lacks the server's mode: %s", name, line)
		}
	}
}
