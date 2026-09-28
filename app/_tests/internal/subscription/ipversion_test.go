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

// A domain export is one link per node in every format, carrying the
// families the domain serves; an address export is a link per family, each
// pinned to its own.
func TestDomainAndAddressExports(t *testing.T) {
	s := &store.Store{SingBox: &store.SingBoxConfig{Inbounds: []store.Inbound{{
		Type: "anytls", Tag: "anytls_2053", ListenPort: 2053,
		Users: []store.User{{Name: "alice", Password: "pw"}},
		TLS:   &store.TLSConfig{Enabled: true, ServerName: "example.com"},
	}}}, UserMeta: store.NewUserManagement()}
	entry := derived.Membership(s)["alice"][0]
	node := proxyName("anytls", "alice", "", false, 0)

	domain := NewRenderer(s, nil, Targets{Host: "example.com", Links: []Target{{Host: "example.com", Family: "dual"}}})
	for format, want := range map[Format]string{FormatSurge: "ip-version=prefer-v4", FormatMihomo: `ip-version: "ipv4-prefer"`} {
		links, err := domain.Render(context.Background(), entry, format)
		if err != nil || len(links) != 1 {
			t.Fatalf("%s: %#v %v", format, links, err)
		}
		if !strings.Contains(links[0].Content, want) || !strings.Contains(links[0].Content, node) || !strings.Contains(links[0].Content, "example.com") {
			t.Fatalf("%s domain link: %s", format, links[0].Content)
		}
	}

	addresses := NewRenderer(s, nil, Targets{Host: "192.0.2.1", Links: []Target{{Host: "192.0.2.1", Family: "v4"}, {Host: "2001:db8::1", Family: "v6"}}})
	for format, wants := range map[Format][2]string{
		FormatSurge:  {"", ""},
		FormatMihomo: {`ip-version: "ipv4"`, `ip-version: "ipv6"`},
	} {
		links, err := addresses.Render(context.Background(), entry, format)
		if err != nil || len(links) != 2 {
			t.Fatalf("%s: %#v %v", format, links, err)
		}
		for i, host := range []string{"192.0.2.1", "2001:db8::1"} {
			content := links[i].Content
			if !strings.Contains(content, host) || !strings.Contains(content, wants[i]) {
				t.Fatalf("%s address link %d: %s", format, i, content)
			}
			if format == FormatSurge && strings.Contains(content, "ip-version") {
				t.Fatalf("surge address line carries ip-version: %s", content)
			}
			if !strings.Contains(content, proxyName("anytls", "alice", links[i].Family, false, 0)) {
				t.Fatalf("%s address link %d is not named for its family: %s", format, i, content)
			}
		}
	}
}
