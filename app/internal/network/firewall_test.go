package network

import (
	"os"
	"path/filepath"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

func TestDesiredFirewallPortsKeepsACMEPortsWhenDomainFileExists(t *testing.T) {
	dir := t.TempDir()
	prevDomainFile := config.DomainFile
	prevCaddyFile := config.CaddyFile
	config.DomainFile = filepath.Join(dir, ".domain")
	config.CaddyFile = filepath.Join(dir, "Caddyfile")
	t.Cleanup(func() {
		config.DomainFile = prevDomainFile
		config.CaddyFile = prevCaddyFile
	})

	if err := os.WriteFile(config.DomainFile, []byte("example.com\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := &store.Store{
		SingBox:      &store.SingBoxConfig{},
		UserMeta:     store.NewUserManagement(),
		UserTemplate: &store.UserRouteTemplates{Templates: map[string][]store.TemplateRule{}},
	}

	specs, err := DesiredFirewallPorts(s)
	if err != nil {
		t.Fatalf("DesiredFirewallPorts error: %v", err)
	}

	var has80 bool
	var has443 bool
	for _, spec := range specs {
		if spec.Proto != "tcp" {
			continue
		}
		if spec.Port == 80 {
			has80 = true
		}
		if spec.Port == 443 {
			has443 = true
		}
	}
	if !has80 || !has443 {
		t.Fatalf("expected tcp/80 and tcp/443 in desired firewall ports, got %#v", specs)
	}
}
