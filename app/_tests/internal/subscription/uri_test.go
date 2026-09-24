package subscription

import (
	"strings"
	"testing"

	"go-proxy/internal/store"
)

func TestRenderURIUsesUniqueFragmentsForMultipleInbounds(t *testing.T) {
	s := &store.Store{
		SingBox: &store.SingBoxConfig{
			Inbounds: []store.Inbound{
				{
					Type:       "anytls",
					Tag:        "anytls_443",
					ListenPort: 443,
					Users: []store.User{
						{Name: "alice", Password: "user-key-1"},
					},
					TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"},
				},
				{
					Type:       "anytls",
					Tag:        "anytls_8443",
					ListenPort: 8443,
					Users: []store.User{
						{Name: "alice", Password: "user-key-2"},
					},
					TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"},
				},
			},
		},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}

	links := renderForUser(t, s, nil, "alice", FormatURI, "1.2.3.4")
	if len(links) != 2 {
		t.Fatalf("len(links) = %d, want 2", len(links))
	}
	if links[0].Content == links[1].Content {
		t.Fatalf("uri links should differ, got %q", links[0].Content)
	}
	// Two nodes of one protocol for one user: the port is what tells their
	// names apart, and it appears only because it has to.
	for _, want := range []string{"#anytls-443-alice", "#anytls-8443-alice"} {
		var found bool
		for _, link := range links {
			if strings.Contains(link.Content, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing uri fragment %q in %#v", want, links)
		}
	}
}
