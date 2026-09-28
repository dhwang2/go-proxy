package subscription

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

func TestExportPreservesTUICCongestionAndChecksCertificates(t *testing.T) {
	ib := store.Inbound{Type: "tuic", Tag: "tuic_443", ListenPort: 443, CongestionControl: "cubic", Users: []store.User{{Name: "alice", UUID: "uuid", Password: "password"}}, TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"}}
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{ib}}, UserMeta: store.NewUserManagement()}
	renderer := NewRenderer(s, nil, Targets{Host: "192.0.2.1", Links: []Target{{Host: "192.0.2.1", Family: "v4"}}})
	entry := derived.Membership(s)["alice"][0]
	for _, format := range []Format{FormatSurge, FormatMihomo} {
		links, err := renderer.Render(context.Background(), entry, format)
		if err != nil {
			t.Fatal(err)
		}
		content := links[0].Content
		// Surge's tuic-v5 has no congestion parameter; the other two carry it.
		if format != FormatSurge && !strings.Contains(content, "cubic") {
			t.Fatalf("%s lost congestion choice", format)
		}
		for _, insecure := range []string{"allow_insecure=1", "insecure=1", "skip-cert-verify=true", "skip-cert-verify: true"} {
			if strings.Contains(content, insecure) {
				t.Fatalf("%s disables certificate checks: %s", format, content)
			}
		}
	}
	s.SingBox.Inbounds[0].Users[0].Password = ""
	if _, err := renderer.Render(context.Background(), entry, FormatMihomo); err == nil {
		t.Fatal("missing password exported successfully")
	}
}

func TestExportNamesDistinguishUsersAndAddressFamilies(t *testing.T) {
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{{Type: "anytls", Tag: "anytls_443", ListenPort: 443, Users: []store.User{{Name: "alice", Password: "pw"}, {Name: "bob", Password: "pw2"}}}}}, UserMeta: store.NewUserManagement()}
	renderer := NewRenderer(s, nil, Targets{Host: "192.0.2.1", Links: []Target{{Host: "192.0.2.1", Family: "v4"}, {Host: "2001:db8::1", Family: "v6"}}})
	seen := map[string]bool{}
	for _, entries := range derived.Membership(s) {
		for _, entry := range entries {
			links, err := renderer.Render(context.Background(), entry, FormatMihomo)
			if err != nil {
				t.Fatal(err)
			}
			for _, link := range links {
				// The client keys its proxy list by name, so two entries
				// sharing one would silently replace each other.
				found := regexp.MustCompile(`name: "([^"]+)"`).FindStringSubmatch(link.Content)
				if found == nil {
					t.Fatalf("entry has no name: %s", link.Content)
				}
				if seen[found[1]] {
					t.Fatalf("duplicate proxy name %s", found[1])
				}
				seen[found[1]] = true
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("got %d entries", len(seen))
	}
}
