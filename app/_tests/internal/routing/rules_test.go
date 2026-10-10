package routing

import (
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

func setupRoutingStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()

	config.SingBoxConfig = filepath.Join(dir, "conf", "sing-box.json")
	config.UserMetaFile = filepath.Join(dir, "user-management.json")
	config.UserRouteFile = filepath.Join(dir, "user-route-rules.json")
	config.FirewallConfigFile = filepath.Join(dir, "firewall-ports.json")
	config.SnellConfigFile = filepath.Join(dir, "snell-v6.conf")
	config.SingBoxBin = "/nonexistent/sing-box"

	if err := os.MkdirAll(filepath.Dir(config.SingBoxConfig), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}

	s, err := store.Load()
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	return s
}

func preset(t *testing.T, name string) Preset {
	t.Helper()
	p, ok := FindPreset(name)
	if !ok {
		t.Fatalf("no preset %q", name)
	}
	return p
}

// A user's rules are selected by preset: add creates once, modify repoints
// only the named presets, remove takes only those, and another user's rule for
// the same preset is never touched.
func TestUserRulesAreSelectedByPreset(t *testing.T) {
	s := setupRoutingStore(t)
	if !AddUserPreset(s, "alice", preset(t, "openai"), "direct") || !AddUserPreset(s, "alice", preset(t, "google"), "direct") ||
		!AddUserPreset(s, "bob", preset(t, "openai"), "direct") {
		t.Fatal("a new preset was not added")
	}
	if AddUserPreset(s, "alice", preset(t, "openai"), "res1") {
		t.Fatal("a preset the user has was added again")
	}
	if !s.IsDirtyFile(store.FileUserRoutes) {
		t.Fatal("an added rule was not marked for persistence")
	}
	if got := SetUserPresetsOutbound(s, "alice", []string{"openai"}, "res1"); got != 1 {
		t.Fatalf("repointed %d rules, want 1", got)
	}
	if got := SetUserPresetsOutbound(s, "alice", []string{"openai"}, "res1"); got != 0 {
		t.Fatalf("repointing to the same outbound changed %d rules", got)
	}
	outbounds := map[string]string{}
	for _, rule := range s.UserRoutes {
		p, _ := PresetOf(rule)
		outbounds[rule.AuthUser[0]+"/"+p.Name] = rule.Outbound
	}
	if outbounds["alice/openai"] != "res1" || outbounds["alice/google"] != "direct" || outbounds["bob/openai"] != "direct" {
		t.Fatalf("outbounds after modify: %v", outbounds)
	}
	if got := RemoveUserPresets(s, "alice", []string{"openai"}); got != 1 || len(s.UserRoutes) != 2 {
		t.Fatalf("removed %d, left %#v", got, s.UserRoutes)
	}
	if got := ClearUser(s, "alice"); got != 1 || len(s.UserRoutes) != 1 || s.UserRoutes[0].AuthUser[0] != "bob" {
		t.Fatalf("clear removed %d, left %#v", got, s.UserRoutes)
	}
	if got := ClearAll(s); got != 1 || len(s.UserRoutes) != 0 {
		t.Fatalf("clear all removed %d, left %#v", got, s.UserRoutes)
	}
}

// A stored rule matching no preset is not one gproxy wrote: it compiles to
// nothing rather than to a rule that names rule sets the catalogue may lack.
func TestRuleMatchingNoPresetCompilesToNothing(t *testing.T) {
	s := setupRoutingStore(t)
	s.UserRoutes = []store.UserRouteRule{{Outbound: "direct", AuthUser: []string{"alice"}, RuleSet: []string{"geosite-unknown"}}}
	if rules := CompiledUserRouteRules(s); len(rules) != 0 {
		t.Fatalf("compiled %#v", rules)
	}
}
