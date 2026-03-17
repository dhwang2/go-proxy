package config

import (
	"encoding/json"
	"os"

	"go-proxy/pkg/fileutil"
)

// defaultSingBoxConfig is the initial sing-box configuration generated on first use.
// It matches the shell-proxy initial config: log + dns + empty inbounds + direct outbound + basic route.
var defaultSingBoxConfig = map[string]any{
	"log": map[string]any{
		"level":     "warn",
		"timestamp": true,
	},
	"dns": map[string]any{
		"servers": []map[string]any{
			{
				"tag":         "public4",
				"type":        "https",
				"server":      "8.8.8.8",
				"server_port": 443,
				"path":        "/dns-query",
				"tls": map[string]any{
					"enabled":     true,
					"server_name": "dns.google",
				},
			},
			{
				"tag":         "public6",
				"type":        "https",
				"server":      "2001:4860:4860::8888",
				"server_port": 443,
				"path":        "/dns-query",
				"tls": map[string]any{
					"enabled":     true,
					"server_name": "dns.google",
				},
			},
			{
				"tag":         "cloudflare",
				"type":        "https",
				"server":      "1.1.1.1",
				"server_port": 443,
				"path":        "/dns-query",
				"tls": map[string]any{
					"enabled":     true,
					"server_name": "cloudflare-dns.com",
				},
			},
			{
				"tag":         "dns-direct",
				"type":        "https",
				"server":      "8.8.8.8",
				"server_port": 443,
				"path":        "/dns-query",
				"tls": map[string]any{
					"enabled":     true,
					"server_name": "dns.google",
				},
			},
		},
		"rules": []any{},
	},
	"inbounds":  []any{},
	"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
	"route": map[string]any{
		"default_domain_resolver": "dns-direct",
		"rules":                   []any{},
	},
}

// Bootstrap creates the runtime directories and default configuration files
// if they do not already exist. It should be called on first run.
func Bootstrap() error {
	// Create directories.
	for _, dir := range []string{WorkDir, ConfDir, BinDir, LogDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Create default sing-box.json if missing.
	if _, err := os.Stat(SingBoxConfig); os.IsNotExist(err) {
		data, err := json.MarshalIndent(defaultSingBoxConfig, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := fileutil.AtomicWrite(SingBoxConfig, data); err != nil {
			return err
		}
	}

	return nil
}
