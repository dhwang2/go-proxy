package subscription

import (
	"context"
	"net"
	"net/url"
	"regexp"
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
	for _, format := range []Format{FormatURI, FormatSurge, FormatMihomo} {
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
	renderer := NewRenderer(s, nil, "192.0.2.1", []SurgeTarget{{Host: "192.0.2.1", Family: "v4"}, {Host: "2001:db8::1", Family: "v6"}})
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

// Share links follow the schemes Mihomo parses: percent-encoded credentials
// and names, the empty path written as "/", and only the parameters the
// scheme defines. A password holding '@' or ':' used to end the authority.
func TestShareLinksParseBackToTheirParts(t *testing.T) {
	tuic := store.Inbound{Type: "tuic", Tag: "tuic_443", ListenPort: 443, Users: []store.User{{Name: "alice", UUID: "uuid-1", Password: "p@ss:w/rd"}}, TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"}}
	anytls := store.Inbound{Type: "anytls", Tag: "anytls_8443", ListenPort: 8443, Users: []store.User{{Name: "alice", Password: "a@b:c"}}, TLS: &store.TLSConfig{Enabled: true, ServerName: "example.com"}}
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{tuic, anytls}}, UserMeta: store.NewUserManagement()}
	renderer := NewRenderer(s, nil, "2001:db8::1", []SurgeTarget{{Host: "2001:db8::1", Family: "v6"}})
	for _, entry := range derived.Membership(s)["alice"] {
		links, err := renderer.Render(context.Background(), entry, FormatURI)
		if err != nil {
			t.Fatal(err)
		}
		link, err := url.Parse(links[0].Content)
		if err != nil {
			t.Fatalf("%s: %v", links[0].Content, err)
		}
		if link.Hostname() != "2001:db8::1" || link.Path != "/" || link.Fragment == "" {
			t.Fatalf("host %q path %q name %q in %s", link.Hostname(), link.Path, link.Fragment, links[0].Content)
		}
		password, _ := link.User.Password()
		switch link.Scheme {
		case "tuic":
			if link.User.Username() != "uuid-1" || password != "p@ss:w/rd" {
				t.Fatalf("tuic credentials did not survive: %s", links[0].Content)
			}
			if link.Query().Has("allow_insecure") {
				t.Fatalf("tuic link carries a parameter Mihomo dropped: %s", links[0].Content)
			}
		case "anytls":
			if link.User.Username() != "a@b:c" || link.Query().Get("sni") != "example.com" {
				t.Fatalf("anytls link lost its parts: %s", links[0].Content)
			}
		}
	}
}
