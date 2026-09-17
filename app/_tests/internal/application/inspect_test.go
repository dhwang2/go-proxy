package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/service"
	"go-proxy/internal/store"
)

func TestConfigViewRedactsTypedAndArbitrarySecretsWithoutWriting(t *testing.T) {
	a := protocolTestApp(t)
	configJSON := []byte(`{"inbounds":[{"type":"vless","tag":"test","password":"secret-server","users":[{"name":"alice","uuid":"secret-uuid","password":"secret-user"}],"tls":{"enabled":true,"reality":{"enabled":true,"private_key":"secret-reality"}}}],"outbounds":[{"type":"socks","tag":"relay","password":"secret-relay"}],"dns":{"servers":[{"tag":"test","password":"secret-dns"}]},"route":{"rule_set":[{"tag":"test","headers":{"key":"secret-header"}}]},"experimental":{"nested":[{"PSK":"secret-experimental"}]}}`)
	if err := os.WriteFile(config.SingBoxConfig, configJSON, 0600); err != nil {
		t.Fatal(err)
	}
	for _, secrets := range []bool{false, true} {
		result, err := a.ConfigView(context.Background(), "sing-box", secrets)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"server", "uuid", "user", "reality", "relay", "dns", "header", "experimental"} {
			if strings.Contains(string(encoded), "secret-"+secret) != secrets {
				t.Fatalf("secret inclusion mismatch for %s, show-secrets=%v", secret, secrets)
			}
		}
		if !strings.Contains(string(encoded), "alice") || !strings.Contains(string(encoded), "relay") {
			t.Fatal("inspection lost non-secret configuration")
		}
	}
	current, err := os.ReadFile(config.SingBoxConfig)
	if err != nil || string(current) != string(configJSON) {
		t.Fatal("configuration inspection modified the file")
	}
}

func TestStatusMissingConfiguredServiceIsUnhealthy(t *testing.T) {
	a := protocolTestApp(t)
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{"inbounds":[{"type":"vless","tag":"vless_24443","listen_port":24443}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' 'Id=sing-box.service' 'LoadState=not-found' 'ActiveState=inactive' 'UnitFileState=' '' 'Id=snell-v6.service' 'LoadState=not-found' 'ActiveState=inactive' 'UnitFileState=' '' 'Id=caddy-sub.service' 'LoadState=not-found' 'ActiveState=inactive' 'UnitFileState=' '' 'Id=proxy-watchdog.service' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=enabled'\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := a.Status(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	if data["healthy"] != false {
		t.Fatal("missing configured service was reported healthy")
	}
	if !strings.Contains(strings.Join(data["issues"].([]string), " "), "sing-box") {
		t.Fatalf("missing service omitted from issues: %v", data["issues"])
	}
}

func TestStatusInstalledButStoppedServiceIsExplainedInIssues(t *testing.T) {
	a := protocolTestApp(t)
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{"inbounds":[{"type":"vless","tag":"vless_24443","listen_port":24443}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' 'Id=sing-box.service' 'LoadState=loaded' 'ActiveState=inactive' 'UnitFileState=enabled' '' 'Id=snell-v6.service' 'LoadState=not-found' 'ActiveState=inactive' 'UnitFileState=' '' 'Id=caddy-sub.service' 'LoadState=not-found' 'ActiveState=inactive' 'UnitFileState=' '' 'Id=proxy-watchdog.service' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=enabled'\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := a.Status(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	if data["healthy"] != false {
		t.Fatal("stopped configured service was reported healthy")
	}
	issues := strings.Join(data["issues"].([]string), " ")
	if !strings.Contains(issues, "sing-box") {
		t.Fatalf("stopped service left healthy=false with no issue naming it: %v", data["issues"])
	}
	if strings.Contains(issues, "missing") {
		t.Fatalf("installed but stopped service reported as missing: %v", data["issues"])
	}
}

func TestShadowTLSValidationChecksCoreAndBinding(t *testing.T) {
	a := protocolTestApp(t)
	dir := t.TempDir()
	validator := filepath.Join(dir, "sing-box")
	shadow := filepath.Join(dir, "shadow-tls")
	if err := os.WriteFile(validator, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	oldSing, oldShadow := config.SingBoxBin, config.ShadowTLSBin
	config.SingBoxBin, config.ShadowTLSBin = validator, shadow
	t.Cleanup(func() { config.SingBoxBin = oldSing; config.ShadowTLSBin = oldShadow })
	snapshot, err := a.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Store.SingBox.Inbounds = []store.Inbound{{Type: "shadowsocks", Tag: "ss_8388", ListenPort: 8388}}
	valid := service.ShadowTLSBinding{ListenPort: 8443, BackendPort: 8388, BackendProto: "ss", Password: "not-for-diagnostics", SNI: "www.netbsd.org", Version: 3}
	snapshot.Bindings = []service.ShadowTLSBinding{valid}
	if _, err := validateConfiguration(context.Background(), snapshot); err == nil || !strings.Contains(err.Error(), "core is not installed") {
		t.Fatalf("missing core was accepted: %v", err)
	}
	if err := os.WriteFile(shadow, []byte("binary fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := validateConfiguration(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*service.ShadowTLSBinding){
		func(b *service.ShadowTLSBinding) { b.ListenPort = 65536 },
		func(b *service.ShadowTLSBinding) { b.BackendPort = 65536 },
		func(b *service.ShadowTLSBinding) { b.Version = 2 },
		func(b *service.ShadowTLSBinding) { b.SNI = "invalid host" },
		func(b *service.ShadowTLSBinding) { b.BackendProto = "vless" },
		func(b *service.ShadowTLSBinding) { b.BackendPort = 8389 },
		func(b *service.ShadowTLSBinding) { b.ListenPort = 8388 },
	} {
		binding := valid
		mutate(&binding)
		snapshot.Bindings = []service.ShadowTLSBinding{binding}
		if _, err := validateConfiguration(context.Background(), snapshot); err == nil {
			t.Fatalf("accepted invalid binding: %+v", binding)
		} else if strings.Contains(err.Error(), valid.Password) {
			t.Fatal("credential leaked in validation error")
		}
	}
}

func TestConfigValidationUsesCapturedSnapshot(t *testing.T) {
	a := protocolTestApp(t)
	dir := t.TempDir()
	validator := filepath.Join(dir, "sing-box")
	if err := os.WriteFile(validator, []byte("#!/bin/sh\ngrep -q snapshot-marker \"$3\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	old := config.SingBoxBin
	config.SingBoxBin = validator
	t.Cleanup(func() { config.SingBoxBin = old })
	if err := os.WriteFile(config.SingBoxConfig, []byte(`{"inbounds":[{"type":"vless","tag":"snapshot-marker","listen_port":24443}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.SingBoxConfig, []byte("external change"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateConfiguration(context.Background(), snapshot); err != nil {
		t.Fatalf("validator read live files instead of snapshot: %v", err)
	}
	content, _ := os.ReadFile(config.SingBoxConfig)
	if string(content) != "external change" {
		t.Fatal("query changed persisted state")
	}
}
