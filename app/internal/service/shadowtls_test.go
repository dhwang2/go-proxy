package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseShadowTLSBindingPrefersExplicitBackendMetadata(t *testing.T) {
	unit := `[Unit]
Description=Shadow-TLS v3 Service

[Service]
Type=simple
Environment=GPROXY_SHADOWTLS_BACKEND=ss
Environment=GPROXY_SHADOWTLS_BACKEND_PORT=8388
ExecStart=/etc/go-proxy/bin/shadow-tls --v3 server --listen 0.0.0.0:443 --server 127.0.0.1:8388 --tls www.microsoft.com --password secret
`

	binding, ok := parseShadowTLSBinding("shadow-tls-ss-8388", "/etc/systemd/system/shadow-tls-ss-8388.service", unit)
	if !ok {
		t.Fatal("expected binding to parse")
	}
	if binding.BackendProto != "ss" {
		t.Fatalf("BackendProto = %q, want ss", binding.BackendProto)
	}
	if binding.BackendPort != 8388 {
		t.Fatalf("BackendPort = %d, want 8388", binding.BackendPort)
	}
	if binding.ListenPort != 443 {
		t.Fatalf("ListenPort = %d, want 443", binding.ListenPort)
	}
	if binding.SNI != "www.microsoft.com" {
		t.Fatalf("SNI = %q, want www.microsoft.com", binding.SNI)
	}
	if binding.Password != "secret" {
		t.Fatalf("Password = %q, want secret", binding.Password)
	}
	if binding.Version != 3 {
		t.Fatalf("Version = %d, want 3", binding.Version)
	}
}

func TestRemoveShadowTLSBindingByBackendRemovesMatchingUnit(t *testing.T) {
	dir := t.TempDir()
	prevDir := shadowTLSUnitDir
	shadowTLSUnitDir = dir
	t.Cleanup(func() {
		shadowTLSUnitDir = prevDir
	})

	unitPath := filepath.Join(dir, "shadow-tls-ss-8388.service")
	if err := os.WriteFile(unitPath, []byte(`[Unit]
[Service]
Environment=GPROXY_SHADOWTLS_BACKEND=ss
Environment=GPROXY_SHADOWTLS_BACKEND_PORT=8388
ExecStart=/etc/go-proxy/bin/shadow-tls --v3 server --listen 0.0.0.0:443 --server 127.0.0.1:8388 --tls www.microsoft.com --password secret
`), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	if err := RemoveShadowTLSBindingByBackend(context.Background(), "ss", 8388); err != nil {
		t.Fatalf("RemoveShadowTLSBindingByBackend error: %v", err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit still exists, stat err=%v", err)
	}
}

func TestListShadowTLSBindingsFallsBackToCurrentServiceNamePattern(t *testing.T) {
	dir := t.TempDir()
	prevDir := shadowTLSUnitDir
	shadowTLSUnitDir = dir
	t.Cleanup(func() {
		shadowTLSUnitDir = prevDir
	})

	unitPath := filepath.Join(dir, "shadow-tls-snell-1443.service")
	if err := os.WriteFile(unitPath, []byte(`[Unit]
[Service]
ExecStart=/etc/go-proxy/bin/shadow-tls --v3 server --listen 0.0.0.0:8443 --server 127.0.0.1:1443 --tls www.microsoft.com --password secret
`), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	bindings, err := ListShadowTLSBindings(nil)
	if err != nil {
		t.Fatalf("ListShadowTLSBindings error: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings len = %d, want 1", len(bindings))
	}
	if bindings[0].BackendProto != "snell" {
		t.Fatalf("BackendProto = %q, want snell", bindings[0].BackendProto)
	}
	if bindings[0].ListenPort != 8443 || bindings[0].BackendPort != 1443 {
		t.Fatalf("ports = %d/%d, want 8443/1443", bindings[0].ListenPort, bindings[0].BackendPort)
	}
}
