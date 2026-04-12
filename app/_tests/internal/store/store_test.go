package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
)

func setupTestDir(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()

	// Override config paths to use temp dir.
	config.SingBoxConfig = filepath.Join(dir, "conf", "sing-box.json")
	config.UserMetaFile = filepath.Join(dir, "user-management.json")
	config.UserRouteFile = filepath.Join(dir, "user-route-rules.json")
	config.UserTemplateFile = filepath.Join(dir, "user-route-templates.json")
	config.SnellConfigFile = filepath.Join(dir, "snell-v5.conf")
	config.SingBoxBin = "/nonexistent/sing-box" // skip validation

	os.MkdirAll(filepath.Join(dir, "conf"), 0755)

	return func() {}
}

func TestLoadEmpty(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if s.SingBox == nil {
		t.Fatal("SingBox should not be nil")
	}
	if s.UserMeta == nil {
		t.Fatal("UserMeta should not be nil")
	}
	if s.UserMeta.Schema != 3 {
		t.Errorf("UserMeta.Schema = %d, want 3", s.UserMeta.Schema)
	}
	if s.UserTemplate == nil {
		t.Fatal("UserTemplate should not be nil")
	}
}

func TestRoundtrip(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	// Write initial config.
	sbConfig := &SingBoxConfig{
		Inbounds: []Inbound{
			{
				Type:       "vless",
				Tag:        "vless_8443",
				Listen:     "0.0.0.0",
				ListenPort: 8443,
				Users: []User{
					{Name: "alice", UUID: "test-uuid-1"},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(sbConfig, "", "  ")
	os.WriteFile(config.SingBoxConfig, data, 0644)

	umConfig := NewUserManagement()
	umConfig.Name["vless|vless_8443|test-uuid-1"] = "alice"
	data, _ = json.MarshalIndent(umConfig, "", "  ")
	os.WriteFile(config.UserMetaFile, data, 0644)

	// Load.
	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Verify loaded state.
	if len(s.SingBox.Inbounds) != 1 {
		t.Fatalf("Inbounds count = %d, want 1", len(s.SingBox.Inbounds))
	}
	if s.SingBox.Inbounds[0].Tag != "vless_8443" {
		t.Errorf("Tag = %q, want vless_8443", s.SingBox.Inbounds[0].Tag)
	}
	if s.UserMeta.Name["vless|vless_8443|test-uuid-1"] != "alice" {
		t.Error("UserMeta.Name not loaded correctly")
	}

	// Mutate and save.
	s.SingBox.Inbounds[0].Users = append(s.SingBox.Inbounds[0].Users,
		User{Name: "bob", UUID: "test-uuid-2"})
	s.MarkDirty(FileSingBox)

	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Reload and verify.
	s2, err := Load()
	if err != nil {
		t.Fatalf("Load() after save error: %v", err)
	}
	if len(s2.SingBox.Inbounds[0].Users) != 2 {
		t.Fatalf("Users count = %d, want 2", len(s2.SingBox.Inbounds[0].Users))
	}
	if s2.SingBox.Inbounds[0].Users[1].Name != "bob" {
		t.Errorf("User[1].Name = %q, want bob", s2.SingBox.Inbounds[0].Users[1].Name)
	}
}

func TestSnellRoundtrip(t *testing.T) {
	input := "[snell server]\nlisten = 0.0.0.0:8448\npsk = testpsk123\n"
	conf, err := ParseSnellConfig(input)
	if err != nil {
		t.Fatalf("ParseSnellConfig error: %v", err)
	}
	if conf.Listen != "0.0.0.0:8448" {
		t.Errorf("Listen = %q, want 0.0.0.0:8448", conf.Listen)
	}
	if conf.PSK != "testpsk123" {
		t.Errorf("PSK = %q, want testpsk123", conf.PSK)
	}
	if conf.Obfs != "off" {
		t.Errorf("Obfs = %q, want off", conf.Obfs)
	}

	output := string(conf.MarshalSnellConfig())
	if !strings.HasPrefix(output, "[snell-server]\n") {
		t.Fatalf("MarshalSnellConfig() header = %q, want [snell-server]", output)
	}
	conf2, err := ParseSnellConfig(output)
	if err != nil {
		t.Fatalf("re-parse error: %v", err)
	}
	if conf2.Listen != conf.Listen || conf2.PSK != conf.PSK {
		t.Error("roundtrip mismatch")
	}
	if conf2.Obfs != "off" || conf2.IPv6 || conf2.UDP {
		t.Fatalf("roundtrip snell defaults mismatch: %+v", conf2)
	}
}

func TestUserKey(t *testing.T) {
	key := UserKey("vless", "vless_8443", "test-uuid")
	want := "vless|vless_8443|test-uuid"
	if key != want {
		t.Errorf("UserKey = %q, want %q", key, want)
	}
}

func TestSaveRemovesSnellConfigWhenUnset(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	if err := os.WriteFile(config.SnellConfigFile, []byte("[snell server]\nlisten = 0.0.0.0:8448\npsk = testpsk\n"), 0644); err != nil {
		t.Fatalf("write snell config: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	s.SnellConf = nil
	s.MarkDirty(FileSnellConf)
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if _, err := os.Stat(config.SnellConfigFile); !os.IsNotExist(err) {
		t.Fatalf("snell config should be removed, stat err = %v", err)
	}
}

func TestRoundtripPreservesShellProxyBaselineFields(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	raw := []byte(`{
  "log": {
    "disabled": false,
    "level": "error",
    "output": "/etc/go-proxy/logs/sing-box.log",
    "timestamp": true
  },
  "experimental": {
    "cache_file": {
      "enabled": true
    }
  },
  "dns": {
    "servers": [
      {
        "tag": "public4",
        "type": "https",
        "server": "8.8.8.8",
        "server_port": 443
      }
    ],
    "rules": [],
    "final": "public4",
    "strategy": "ipv4_only",
    "reverse_mapping": true,
    "independent_cache": true,
    "cache_capacity": 8192
  },
  "outbounds": [
    {
      "type": "direct",
      "tag": "🐸 direct"
    }
  ],
  "route": {
    "final": "🐸 direct",
    "default_domain_resolver": "public4",
    "rules": [
      {
        "action": "sniff",
        "sniffer": ["http", "tls", "quic", "dns"]
      },
      {
        "ip_is_private": true,
        "action": "route",
        "outbound": "🐸 direct"
      },
      {
        "protocol": "dns",
        "action": "hijack-dns"
      }
    ],
    "rule_set": []
  }
}`)
	if err := os.WriteFile(config.SingBoxConfig, raw, 0644); err != nil {
		t.Fatalf("write sing-box config: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	s.MarkDirty(FileSingBox)
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(config.SingBoxConfig)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}

	logMap := parsed["log"].(map[string]any)
	if _, ok := logMap["disabled"]; !ok {
		t.Fatal("saved log config is missing disabled")
	}
	if got := logMap["output"]; got != "/etc/go-proxy/logs/sing-box.log" {
		t.Fatalf("log.output = %v, want /etc/go-proxy/logs/sing-box.log", got)
	}

	dnsMap := parsed["dns"].(map[string]any)
	if got := dnsMap["reverse_mapping"]; got != true {
		t.Fatalf("dns.reverse_mapping = %v, want true", got)
	}
	if got := dnsMap["independent_cache"]; got != true {
		t.Fatalf("dns.independent_cache = %v, want true", got)
	}
	if got := dnsMap["cache_capacity"]; got != float64(8192) {
		t.Fatalf("dns.cache_capacity = %v, want 8192", got)
	}

	routeMap := parsed["route"].(map[string]any)
	rules := routeMap["rules"].([]any)
	if len(rules) != 3 {
		t.Fatalf("route.rules len = %d, want 3", len(rules))
	}
	first := rules[0].(map[string]any)
	if _, ok := first["sniffer"]; !ok {
		t.Fatal("first route rule is missing sniffer")
	}
	second := rules[1].(map[string]any)
	if got := second["ip_is_private"]; got != true {
		t.Fatalf("second route rule ip_is_private = %v, want true", got)
	}
	third := rules[2].(map[string]any)
	if got := third["protocol"]; got != "dns" {
		t.Fatalf("third route rule protocol = %v, want dns", got)
	}
}

func TestSaveDeletesSnellConfigWhenUnset(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	if err := os.WriteFile(config.SnellConfigFile, []byte("[snell server]\nlisten = 0.0.0.0:8448\npsk = testpsk\n"), 0644); err != nil {
		t.Fatalf("write snell config: %v", err)
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	s.SnellConf = nil
	s.MarkDirty(FileSnellConf)

	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, err := os.Stat(config.SnellConfigFile); !os.IsNotExist(err) {
		t.Fatalf("snell config still exists after save, err=%v", err)
	}
}
