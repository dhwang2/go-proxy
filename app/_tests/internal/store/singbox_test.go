package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSingBoxNormalizeAddsBaselineSections(t *testing.T) {
	cfg := &SingBoxConfig{}
	cfg.Normalize()

	if cfg.DNS == nil || cfg.DNS.Final != "public4" {
		t.Fatalf("Normalize() dns.final = %q, want public4", cfg.DNS.Final)
	}
	if cfg.DNS.Strategy != "" {
		t.Fatalf("Normalize() changed the explicit default strategy to %q", cfg.DNS.Strategy)
	}
	if cfg.Route == nil || cfg.Route.Final != DirectTag {
		t.Fatalf("Normalize() route.final = %q, want direct", cfg.Route.Final)
	}
	if len(cfg.Route.RuleSet) == 0 {
		t.Fatal("Normalize() should populate route.rule_set")
	}
	if len(cfg.Route.Rules) < 3 {
		t.Fatalf("Normalize() route rules len = %d, want at least 3", len(cfg.Route.Rules))
	}
	if len(cfg.Outbounds) == 0 {
		t.Fatal("Normalize() should populate direct outbound")
	}
	var direct struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(cfg.Outbounds[0], &direct); err != nil {
		t.Fatalf("Normalize() unmarshal direct outbound: %v", err)
	}
	if direct.Tag != DirectTag {
		t.Fatalf("Normalize() direct outbound tag = %q, want direct", direct.Tag)
	}
	if len(cfg.Experimental) == 0 {
		t.Fatal("Normalize() should populate experimental")
	}
}

// A configuration written with the old frog-tagged direct outbound is read as
// the plain tag everywhere the tag appears.
func TestNormalizeRewritesTheLegacyDirectTag(t *testing.T) {
	cfg := &SingBoxConfig{
		Outbounds: []json.RawMessage{json.RawMessage(`{"type":"direct","tag":"🐸 direct"}`)},
		Route: &RouteConfig{
			Final:   LegacyDirectTag,
			Rules:   []RouteRule{{Action: "route", IPIsPrivate: true, Outbound: LegacyDirectTag}},
			RuleSet: []json.RawMessage{json.RawMessage(`{"tag":"geosite-x","type":"remote","format":"binary","url":"https://example.com/x.srs","download_detour":"🐸 direct"}`)},
		},
	}
	cfg.Normalize()
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "🐸") {
		t.Fatalf("legacy tag survived normalisation: %s", encoded)
	}
	if cfg.Route.Final != DirectTag {
		t.Fatalf("route.final = %q", cfg.Route.Final)
	}
	directs := 0
	for _, raw := range cfg.Outbounds {
		if h, _ := ParseOutboundHeader(raw); h.Tag == DirectTag {
			directs++
		}
	}
	if directs != 1 {
		t.Fatalf("direct outbounds = %d, want 1: %s", directs, encoded)
	}
}

// The sections and the entries go-proxy owns are written in the order
// sing-box documents them, not the alphabetical order a map encodes in.
func TestNormalizeWritesSectionsAndOwnedEntriesInOrder(t *testing.T) {
	cfg := &SingBoxConfig{}
	cfg.Normalize()
	cfg.SetDirectResolver("public4", "prefer_ipv4")
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	order := []string{`"log"`, `"experimental"`, `"dns"`, `"outbounds"`, `"route"`}
	last := -1
	for _, key := range order {
		at := strings.Index(text, key)
		if at <= last {
			t.Fatalf("section %s out of order in %s", key, text)
		}
		last = at
	}
	for _, want := range []string{
		`{"tag":"public4","type":"https","server":"8.8.8.8","server_port":443,"path":"/dns-query","tls":{"enabled":true,"server_name":"dns.google"}}`,
		`{"type":"direct","tag":"direct","domain_resolver":{"server":"public4","strategy":"prefer_ipv4"}}`,
		`"cache_file":{"enabled":true,"cache_id":"cache.db"`,
		`{"tag":"geosite-openai","type":"remote","format":"binary","url":`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s", want)
		}
	}
	if server, strategy, ok := cfg.DirectResolver(); !ok || server != "public4" || strategy != "prefer_ipv4" {
		t.Fatalf("DirectResolver() = %q %q %v", server, strategy, ok)
	}
}
