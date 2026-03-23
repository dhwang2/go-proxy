package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"go-proxy/pkg/fileutil"
)

// DefaultExperimentalConfig returns the shell-proxy-aligned experimental cache block.
func DefaultExperimentalConfig() map[string]any {
	return map[string]any{
		"cache_file": map[string]any{
			"enabled":      true,
			"cache_id":     "cache.db",
			"path":         filepath.Join(WorkDir, "cache.db"),
			"store_fakeip": false,
			"store_rdrc":   true,
		},
	}
}

// DefaultDNSServers returns the baseline DNS servers for generated sing-box configs.
func DefaultDNSServers() []map[string]any {
	return []map[string]any{
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
	}
}

// DefaultRuleSetCatalog returns the remote rule_set catalog backing routing presets.
func DefaultRuleSetCatalog() []map[string]any {
	specs := []struct {
		tag string
		url string
	}{
		{"geosite-openai", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/openai.srs"},
		{"geosite-anthropic", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/anthropic.srs"},
		{"geosite-category-ai-!cn", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/category-ai-!cn.srs"},
		{"geosite-ai-!cn", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/category-ai-!cn.srs"},
		{"geoip-ai", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/ai.srs"},
		{"geosite-google", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/google.srs"},
		{"geosite-netflix", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/netflix.srs"},
		{"geosite-disney", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/disney.srs"},
		{"geosite-mytvsuper", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/mytvsuper.srs"},
		{"geosite-youtube", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/youtube.srs"},
		{"geosite-spotify", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/spotify.srs"},
		{"geosite-tiktok", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/tiktok.srs"},
		{"geosite-telegram", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/telegram.srs"},
		{"geosite-category-ads-all", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/category-ads-all.srs"},
		{"geosite-twitter", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/twitter.srs"},
		{"geosite-whatsapp", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/whatsapp.srs"},
		{"geosite-facebook", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/facebook.srs"},
		{"geosite-discord", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/discord.srs"},
		{"geosite-instagram", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/instagram.srs"},
		{"geosite-reddit", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/reddit.srs"},
		{"geosite-linkedin", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/linkedin.srs"},
		{"geosite-paypal", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/paypal.srs"},
		{"geosite-microsoft", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/microsoft.srs"},
		{"geosite-xai", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/xai.srs"},
		{"geosite-meta", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/meta.srs"},
		{"geosite-messenger", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/messenger.srs"},
		{"geosite-github", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/github.srs"},
		{"geoip-google", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/google.srs"},
		{"geoip-netflix", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/netflix.srs"},
		{"geoip-twitter", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/twitter.srs"},
		{"geoip-telegram", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/telegram.srs"},
		{"geoip-facebook", "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/facebook.srs"},
	}

	items := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		items = append(items, map[string]any{
			"tag":             spec.tag,
			"type":            "remote",
			"format":          "binary",
			"url":             spec.url,
			"download_detour": "🐸 direct",
		})
	}
	return items
}

// DefaultSingBoxConfig returns the initial sing-box configuration generated on first use.
func DefaultSingBoxConfig() map[string]any {
	return map[string]any{
		"log": map[string]any{
			"disabled":  false,
			"level":     "error",
			"output":    SingBoxLog,
			"timestamp": true,
		},
		"experimental": DefaultExperimentalConfig(),
		"dns": map[string]any{
			"servers":           DefaultDNSServers(),
			"rules":             []any{},
			"final":             "public4",
			"strategy":          "prefer_ipv4",
			"reverse_mapping":   true,
			"independent_cache": true,
			"cache_capacity":    8192,
		},
		"inbounds": []any{},
		"outbounds": []map[string]any{
			{"type": "direct", "tag": "🐸 direct"},
		},
		"route": map[string]any{
			"final":                   "🐸 direct",
			"default_domain_resolver": "public4",
			"rules": []map[string]any{
				{"action": "sniff", "sniffer": []string{"http", "tls", "quic", "dns"}},
				{"protocol": "dns", "action": "hijack-dns"},
				{"ip_is_private": true, "action": "route", "outbound": "🐸 direct"},
			},
			"rule_set": DefaultRuleSetCatalog(),
		},
	}
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
		data, err := json.MarshalIndent(DefaultSingBoxConfig(), "", "  ")
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
