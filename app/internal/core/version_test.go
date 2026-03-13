package core

import "testing"

func TestExtractVersion(t *testing.T) {
	cases := map[string]string{
		"sing-box version 1.11.0":     "1.11.0",
		"snell-server v5.0.0":         "5.0.0",
		"v2.9.1 h1:abcdef":            "2.9.1",
		"shadow-tls version unknown":  "",
		"":                            "",
	}
	for raw, want := range cases {
		if got := extractVersion(raw); got != want {
			t.Fatalf("extractVersion(%q)=%q want %q", raw, got, want)
		}
	}
}
