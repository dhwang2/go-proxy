package subscription

import (
	"context"
	"strings"
	"testing"

	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// A link to the domain may use the families the domain publishes and the
// server has; one side alone is left out and said, and no family in common
// is an error rather than a link that cannot connect.
func TestDomainFamilyIsWhatDNSAndServerShare(t *testing.T) {
	for _, c := range []struct {
		records, server []string
		family          string
		notes           int
		fails           bool
	}{
		{[]string{"v4", "v6"}, []string{"v4", "v6"}, "dual", 0, false},
		{[]string{"v4"}, []string{"v4"}, "v4", 0, false},
		{[]string{"v6"}, []string{"v6"}, "v6", 0, false},
		{[]string{"v4"}, []string{"v4", "v6"}, "v4", 1, false},
		{[]string{"v4", "v6"}, []string{"v6"}, "v6", 1, false},
		{[]string{"v4", "v6"}, nil, "dual", 0, false},
		{[]string{"v4"}, []string{"v6"}, "", 2, true},
	} {
		family, notes, err := domainFamily(c.records, c.server)
		if family != c.family || len(notes) != c.notes || (err != nil) != c.fails {
			t.Fatalf("records %v server %v: %q %q %v", c.records, c.server, family, notes, err)
		}
	}
}

// Surge's ip-version goes on a line whose server is a domain, preferring
// IPv4 where both work; an address line goes without, as Surge applies it
// only to a domain.
func TestSurgeIPVersionFollowsTheDomainsFamilies(t *testing.T) {
	line := "n = anytls, example.com, 443, password=p"
	for target, want := range map[Target]string{
		{Host: "example.com", Family: "dual"}: line + ", ip-version=prefer-v4",
		{Host: "example.com", Family: "v4"}:   line + ", ip-version=v4-only",
		{Host: "example.com", Family: "v6"}:   line + ", ip-version=v6-only",
		{Host: "example.com"}:                 line,
		{Host: "192.0.2.1", Family: "v4"}:     line,
	} {
		if got := withSurgeIPVersion(line, target); got != want {
			t.Fatalf("%+v: %q", target, got)
		}
	}
}

// A node gets one link per format that names a domain, and one mihomo entry
// per server family joined by a fallback group under the node's own name.
func TestOneDomainLinkAndAFallbackGroupPerNode(t *testing.T) {
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{{
		Type: "anytls", Tag: "anytls_2053", ListenPort: 2053,
		Users: []store.User{{Name: "alice", Password: "pw"}},
		TLS:   &store.TLSConfig{Enabled: true, ServerName: "example.com"},
	}}}, UserMeta: store.NewUserManagement()}
	entry := derived.Membership(s)["alice"][0]
	dual := Targets{
		Host:   "example.com",
		Links:  []Target{{Host: "example.com", Family: "dual"}},
		Mihomo: []Target{{Host: "192.0.2.1", Family: "v4"}, {Host: "2001:db8::1", Family: "v6"}},
	}
	renderer := NewRenderer(s, nil, dual)
	node := proxyName("anytls", "alice", "", false, 0)
	for _, format := range []Format{FormatSurge, FormatURI} {
		links, err := renderer.Render(context.Background(), entry, format)
		if err != nil || len(links) != 1 || links[0].Name != node || !strings.Contains(links[0].Content, "example.com") {
			t.Fatalf("%s: %#v %v", format, links, err)
		}
	}
	links, err := renderer.Render(context.Background(), entry, FormatMihomo)
	if err != nil || len(links) != 2 {
		t.Fatalf("mihomo: %#v %v", links, err)
	}
	for i, want := range []string{`server: "192.0.2.1"`, `server: "2001:db8::1"`} {
		if !strings.Contains(links[i].Content, want) || strings.Contains(links[i].Content, "example.com\",port") {
			t.Fatalf("mihomo entry %d: %s", i, links[i].Content)
		}
	}
	if !strings.Contains(links[0].Content, `ip-version: "ipv4"`) || !strings.Contains(links[1].Content, `ip-version: "ipv6"`) {
		t.Fatalf("mihomo entries not pinned to their family: %s / %s", links[0].Content, links[1].Content)
	}
	group := renderer.MihomoGroup(entry, links)
	want := `{name: "` + node + `",type: "fallback",proxies: ["` + links[0].Name + `", "` + links[1].Name + `"],url: "http://www.gstatic.com/generate_204",interval: 300,lazy: true}`
	if group != want {
		t.Fatalf("group:\n%s\nwant:\n%s", group, want)
	}

	// One family, or an explicit domain, is one mihomo entry and no group.
	single := Targets{Host: "example.com", Links: dual.Links, Mihomo: []Target{{Host: "example.com", Family: "dual"}}}
	renderer = NewRenderer(s, nil, single)
	links, err = renderer.Render(context.Background(), entry, FormatMihomo)
	if err != nil || len(links) != 1 || links[0].Name != node || !strings.Contains(links[0].Content, `ip-version: "ipv4-prefer"`) {
		t.Fatalf("single mihomo: %#v %v", links, err)
	}
	if group := renderer.MihomoGroup(entry, links); group != "" {
		t.Fatalf("a single entry got a group: %s", group)
	}
}
