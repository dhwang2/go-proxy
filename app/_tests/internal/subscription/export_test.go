package subscription

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

func TestExplicitIPDoesNotResolveDNS(t *testing.T) {
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("explicit ip triggered dns")
		return nil, nil
	}}
	t.Cleanup(func() { net.DefaultResolver = old })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	host, targets, err := ResolveTargets(ctx, "2001:db8::1", true)
	if err != nil || host != "2001:db8::1" || len(targets) != 1 || targets[0].Family != "v6" {
		t.Fatalf("target: %s %#v %v", host, targets, err)
	}
}

func TestExportPreservesTUICCongestionAndChecksCertificates(t *testing.T) {
	ib := store.Inbound{Type: "tuic", Tag: "tuic_443", ListenPort: 443, CongestionControl: "cubic", Users: []store.User{{Name: "alice", UUID: "uuid", Password: "password"}}, TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"}}
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{ib}}, UserMeta: store.NewUserManagement()}
	renderer := NewRenderer(s, nil, "192.0.2.1", []SurgeTarget{{Host: "192.0.2.1", Family: "v4"}})
	entry := derived.Membership(s)["alice"][0]
	for _, format := range []Format{FormatURI, FormatSurge, FormatSingBox} {
		links, err := renderer.Render(context.Background(), entry, format)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(links[0].Content, "cubic") {
			t.Fatalf("%s lost congestion choice", format)
		}
		if format == FormatURI && !strings.Contains(links[0].Content, "allow_insecure=0") {
			t.Fatal("uri disables certificate checks")
		}
	}
	s.SingBox.Inbounds[0].Users[0].Password = ""
	if _, err := renderer.Render(context.Background(), entry, FormatSingBox); err == nil {
		t.Fatal("missing password exported successfully")
	}
}

func TestExportNamesDistinguishUsersAndAddressFamilies(t *testing.T) {
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{{Type: "anytls", Tag: "anytls_443", ListenPort: 443, Users: []store.User{{Name: "alice", Password: "pw"}, {Name: "bob", Password: "pw2"}}}}}, UserMeta: store.NewUserManagement()}
	renderer := NewRenderer(s, nil, "192.0.2.1", []SurgeTarget{{Host: "192.0.2.1", Family: "v4"}, {Host: "2001:db8::1", Family: "v6"}})
	seen := map[string]bool{}
	for _, entries := range derived.Membership(s) {
		for _, entry := range entries {
			links, err := renderer.Render(context.Background(), entry, FormatSingBox)
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				var out map[string]any
				if err = json.Unmarshal([]byte(link.Content), &out); err != nil {
					t.Fatal(err)
				}
				tag := out["tag"].(string)
				if seen[tag] {
					t.Fatalf("duplicate outbound tag %s", tag)
				}
				seen[tag] = true
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("got %d outbounds", len(seen))
	}
}
