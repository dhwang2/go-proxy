package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"go-proxy/internal/cert"
	"go-proxy/internal/network"
	"go-proxy/internal/service"
)

func statusFields() map[string]any {
	expires := time.Now().Add(89 * 24 * time.Hour)
	return map[string]any{
		"healthy": true, "complete": true, "issues": []string{},
		"users": 2, "nodes": 5, "routing_rules": 12,
		"system": map[string]any{"os": "linux", "arch": "amd64"},
		"cert":   cert.Status{Domain: "example.com", Ready: true, ExpiresAt: &expires},
		"network": network.Observation{Addresses: []network.Address{
			{Family: "ipv4", Scope: "loopback"},
			{Family: "ipv4", Scope: "global"},
			{Family: "ipv6", Scope: "link_local"},
		}},
		"services": []service.Status{
			{Name: service.SingBox, Installed: true, Running: true, State: "active"},
			{Name: service.Snell, Installed: true, Running: false, State: "inactive"},
			{Name: service.ShadowTLS, Installed: false},
		},
	}
}

func TestStatusRenderingReportsEveryDashboardRow(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy status", statusFields()) {
		t.Fatal("status was not rendered")
	}
	text := out.String()
	for _, want := range []string{
		"system", "linux", "amd64",
		"network", "IPv4 ok", "IPv6 none",
		"protocol", "5 nodes", "2 users", "12 routes",
		"services", "sing-box running", "snell-v6 inactive", "shadow-tls absent",
		"cert", "example.com", "expires in 89 days",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("rendered status is missing %q:\n%s", want, text)
		}
	}
	// A link-local-only family must not be reported as a working stack, and a
	// loopback address must not stand in for a real one.
	if strings.Contains(text, "IPv6 ok") {
		t.Fatalf("link-local address counted as an IPv6 stack:\n%s", text)
	}
}

func TestStatusRenderingSurfacesIssues(t *testing.T) {
	fields := statusFields()
	fields["healthy"] = false
	fields["issues"] = []string{"configured service sing-box is inactive"}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", fields)
	if !strings.Contains(out.String(), "configured service sing-box is inactive") {
		t.Fatalf("issue missing from rendered status:\n%s", out.String())
	}
}

func TestRenderingEmitsNoEscapesWithoutColour(t *testing.T) {
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", statusFields())
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatalf("escape sequence emitted with colour off:\n%q", out.String())
	}
	var coloured bytes.Buffer
	render(&coloured, palette{on: true}, "gproxy status", statusFields())
	if !bytes.Contains(coloured.Bytes(), []byte(ansiOK)) {
		t.Fatal("colour enabled but no escape sequence emitted")
	}
	if strings.Count(coloured.String(), ansiReset) < strings.Count(coloured.String(), "\x1b[38") {
		t.Fatal("a colour was opened without being reset")
	}
}

func TestColourIsOffForAnythingButATerminal(t *testing.T) {
	var buffer bytes.Buffer
	if colorEnabled(&buffer, false) {
		t.Fatal("colour enabled for a non-file writer")
	}
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if colorEnabled(file, false) {
		t.Fatal("colour enabled for a regular file")
	}
	t.Setenv("NO_COLOR", "1")
	if colorEnabled(os.Stdout, false) {
		t.Fatal("NO_COLOR was ignored")
	}
}

func TestRenderingStripsControlCharactersFromSystemText(t *testing.T) {
	fields := statusFields()
	fields["issues"] = []string{"unit \x1b[31mred\x1b[0m failed"}
	fields["services"] = []service.Status{{Name: service.Name("sing-box\x1b[5m"), Installed: true, Running: true, State: "active"}}
	var out bytes.Buffer
	render(&out, palette{}, "gproxy status", fields)
	if bytes.Contains(out.Bytes(), []byte{0x1b}) {
		t.Fatalf("escape sequence from system text survived into colour-free output:\n%q", out.String())
	}
	if !strings.Contains(out.String(), "red") || !strings.Contains(out.String(), "sing-box") {
		t.Fatalf("stripping removed legible text:\n%s", out.String())
	}
}

func TestStatusRenderingSkipsRowsWithNoData(t *testing.T) {
	var out bytes.Buffer
	if !render(&out, palette{}, "gproxy status", map[string]any{"healthy": true}) {
		t.Fatal("status was not rendered")
	}
	for _, absent := range []string{"nodes", "users", "routes", "system", "cert"} {
		if strings.Contains(out.String(), absent) {
			t.Fatalf("row %q rendered with no data behind it:\n%q", absent, out.String())
		}
	}
}

func TestUnrenderedCommandsFallBackToJSON(t *testing.T) {
	var out bytes.Buffer
	if render(&out, palette{}, "gproxy user", map[string]any{"users": []string{"alice"}}) {
		t.Fatal("a command with no rendering claimed to render")
	}
	if out.Len() != 0 {
		t.Fatalf("fallback path wrote output: %q", out.String())
	}
}

func TestCertRenderingFlagsExpiryStates(t *testing.T) {
	soon := time.Now().Add(3 * 24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	cases := []struct {
		status cert.Status
		want   string
	}{
		{cert.Status{Domain: "a.example", Ready: false}, "not issued"},
		{cert.Status{Domain: "a.example", Ready: true}, "ready"},
		{cert.Status{Domain: "a.example", Ready: true, ExpiresAt: &soon}, "expires in 3 days"},
		{cert.Status{Domain: "a.example", Ready: true, ExpiresAt: &past}, "expired"},
	}
	for _, testCase := range cases {
		if got := renderCert(palette{}, testCase.status); !strings.Contains(got, testCase.want) {
			t.Fatalf("cert rendering %+v produced %q, want %q", testCase.status, got, testCase.want)
		}
	}
}
