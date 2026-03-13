package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPathsFallsBackToLegacyConfig(t *testing.T) {
	workDir := t.TempDir()
	confDir := filepath.Join(workDir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(confDir, "sing-box.json")
	if err := os.WriteFile(legacy, []byte(`{"inbounds":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := DefaultPaths(workDir)
	if paths.ConfPath != legacy {
		t.Fatalf("expected legacy config path %s, got %s", legacy, paths.ConfPath)
	}
}

func TestLoadMissingFilesUsesDefaults(t *testing.T) {
	workDir := t.TempDir()
	paths := DefaultPaths(workDir)
	st, err := Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if st.Config == nil {
		t.Fatalf("expected config to be initialized")
	}
	if st.UserMeta == nil || st.UserMeta.Schema != 3 {
		t.Fatalf("expected default user meta schema=3")
	}
	if st.SnellConf == nil || st.SnellConf.Values == nil {
		t.Fatalf("expected default snell config map")
	}
}

func TestLoadEmptyFilesUsesDefaults(t *testing.T) {
	workDir := t.TempDir()
	paths := DefaultPaths(workDir)
	if err := os.MkdirAll(filepath.Dir(paths.ConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{paths.ConfPath, paths.UserMetaPath, paths.SnellPath} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st, err := Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if st.Config == nil || len(st.Config.Inbounds) != 0 {
		t.Fatalf("expected empty config state")
	}
	if st.UserMeta == nil || st.UserMeta.Schema != 3 {
		t.Fatalf("expected default user meta schema=3")
	}
	if st.SnellConf == nil || len(st.SnellConf.Values) != 0 {
		t.Fatalf("expected empty snell values map")
	}
}

func TestLoadCorruptedConfigJSON(t *testing.T) {
	workDir := t.TempDir()
	paths := DefaultPaths(workDir)
	if err := os.MkdirAll(filepath.Dir(paths.ConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConfPath, []byte(`{"inbounds":[`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "decode config") {
		t.Fatalf("expected decode config error, got: %v", err)
	}
}

func TestLoadCorruptedUserMetaJSON(t *testing.T) {
	workDir := t.TempDir()
	paths := DefaultPaths(workDir)
	if err := os.MkdirAll(filepath.Dir(paths.ConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConfPath, []byte(`{"inbounds":[],"outbounds":[],"route":{},"dns":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.UserMetaPath, []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "decode user meta") {
		t.Fatalf("expected decode user meta error, got: %v", err)
	}
}

func TestLoadLegacyNameObjectShape(t *testing.T) {
	workDir := t.TempDir()
	paths := DefaultPaths(workDir)
	if err := os.MkdirAll(filepath.Dir(paths.ConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConfPath, []byte(`{"inbounds":[],"outbounds":[],"route":{},"dns":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{
		"schema": 2,
		"name": {
			"u-1": {"value": "alpha-user"}
		}
	}`
	if err := os.WriteFile(paths.UserMetaPath, []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := st.UserMeta.Name["u-1"]; got != "alpha-user" {
		t.Fatalf("expected migrated legacy name value, got %q", got)
	}
	if st.UserMeta.Schema != 3 {
		t.Fatalf("expected schema upgraded to 3, got %d", st.UserMeta.Schema)
	}
}

func TestLoadLegacyInboundSingleUserShape(t *testing.T) {
	workDir := t.TempDir()
	paths := DefaultPaths(workDir)
	if err := os.MkdirAll(filepath.Dir(paths.ConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
		"inbounds": [
			{
				"type": "vless",
				"tag": "vless-in",
				"listen_port": 443,
				"uuid": "11111111-1111-1111-1111-111111111111",
				"name": "legacy-user"
			}
		],
		"outbounds": [],
		"route": {},
		"dns": {}
	}`
	if err := os.WriteFile(paths.ConfPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Load(paths.ConfPath, paths.UserMetaPath, paths.SnellPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(st.Config.Inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(st.Config.Inbounds))
	}
	users := st.Config.Inbounds[0].Users
	if len(users) != 1 {
		t.Fatalf("expected legacy single user to migrate into users array, got %d", len(users))
	}
	if users[0].ID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("expected migrated user id from uuid, got %q", users[0].ID)
	}
}
