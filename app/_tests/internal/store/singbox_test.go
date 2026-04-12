package store

import (
	"encoding/json"
	"testing"
)

func TestSingBoxNormalizeAddsBaselineSections(t *testing.T) {
	cfg := &SingBoxConfig{}
	cfg.Normalize()

	if cfg.DNS == nil || cfg.DNS.Final != "public4" {
		t.Fatalf("Normalize() dns.final = %q, want public4", cfg.DNS.Final)
	}
	if cfg.DNS.Strategy != "ipv4_only" {
		t.Fatalf("Normalize() dns.strategy = %q, want ipv4_only", cfg.DNS.Strategy)
	}
	if cfg.Route == nil || cfg.Route.Final != "🐸 direct" {
		t.Fatalf("Normalize() route.final = %q, want 🐸 direct", cfg.Route.Final)
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
	if direct.Tag != "🐸 direct" {
		t.Fatalf("Normalize() direct outbound tag = %q, want 🐸 direct", direct.Tag)
	}
	if len(cfg.Experimental) == 0 {
		t.Fatal("Normalize() should populate experimental")
	}
}
