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
					Type:       "shadowsocks",
					Tag:        "shadowsocks_443",
					ListenPort: 443,
					Method:     "2022-blake3-aes-128-gcm",
					Password:   "server-key-1",
					Users: []store.User{
						{Name: "alice", Password: "user-key-1"},
					},
				},
				{
					Type:       "shadowsocks",
					Tag:        "shadowsocks_8443",
					ListenPort: 8443,
					Method:     "2022-blake3-aes-128-gcm",
					Password:   "server-key-2",
					Users: []store.User{
						{Name: "alice", Password: "user-key-2"},
					},
				},
			},
		},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}

	links := Render(s, "alice", FormatURI, "1.2.3.4")
	if len(links) != 2 {
		t.Fatalf("len(links) = %d, want 2", len(links))
	}
	if links[0].Content == links[1].Content {
		t.Fatalf("uri links should differ, got %q", links[0].Content)
	}
	for _, want := range []string{"shadowsocks_443", "shadowsocks_8443"} {
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
