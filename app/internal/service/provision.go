package service

import (
	"context"
	"fmt"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/pkg/fileutil"
)

// unitSpec describes a systemd unit. Optional fields are empty by default.
type unitSpec struct {
	Description   string
	Documentation string
	After         string
	Environment   []string
	ExecStart     string
	RestartSec    string
	LimitNOFILE   string
	LogPath       string
}

func renderUnit(s unitSpec) string {
	if s.After == "" {
		s.After = "network.target"
	}
	if s.RestartSec == "" {
		s.RestartSec = "10s"
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n", s.Description)
	if s.Documentation != "" {
		fmt.Fprintf(&b, "Documentation=%s\n", s.Documentation)
	}
	fmt.Fprintf(&b, "After=%s\n\n[Service]\nType=simple\n", s.After)
	for _, env := range s.Environment {
		fmt.Fprintf(&b, "Environment=%s\n", env)
	}
	fmt.Fprintf(&b, "ExecStart=%s\nRestart=on-failure\nRestartSec=%s\n", s.ExecStart, s.RestartSec)
	if s.LimitNOFILE != "" {
		fmt.Fprintf(&b, "LimitNOFILE=%s\n", s.LimitNOFILE)
	}
	fmt.Fprintf(&b, "StandardOutput=append:%s\nStandardError=append:%s\n\n[Install]\nWantedBy=multi-user.target\n", s.LogPath, s.LogPath)
	return b.String()
}

// provisionUnit writes a systemd unit file and reloads the daemon.
func provisionUnit(ctx context.Context, path, content string) error {
	if err := fileutil.AtomicWrite(path, []byte(content)); err != nil {
		return err
	}
	return DaemonReload(ctx)
}

// ProvisionSingBox writes the sing-box systemd unit file.
func ProvisionSingBox(ctx context.Context) error {
	return provisionUnit(ctx, config.SingBoxService, renderUnit(unitSpec{
		Description:   "sing-box service",
		Documentation: "https://sing-box.sagernet.org",
		After:         "network.target nss-lookup.target",
		ExecStart:     fmt.Sprintf("%s run -c %s", config.SingBoxBin, config.SingBoxConfig),
		LimitNOFILE:   "infinity",
		LogPath:       config.SingBoxLog,
	}))
}

// ProvisionSnell writes the snell systemd unit file.
func ProvisionSnell(ctx context.Context) error {
	return provisionUnit(ctx, config.SnellService, renderUnit(unitSpec{
		Description: "Snell v5 Proxy Service",
		ExecStart:   fmt.Sprintf("%s -c %s", config.SnellBin, config.SnellConfigFile),
		LogPath:     config.SnellLog,
	}))
}

// ProvisionCaddySub writes the caddy subscription server systemd unit file.
func ProvisionCaddySub(ctx context.Context) error {
	return provisionUnit(ctx, config.CaddySubService, renderUnit(unitSpec{
		Description: "Caddy Subscription Server",
		Environment: []string{"XDG_DATA_HOME=/etc/go-proxy", "XDG_CONFIG_HOME=/etc/go-proxy"},
		ExecStart:   fmt.Sprintf("%s run --config %s --adapter caddyfile", config.CaddyBin, config.CaddyFile),
		LogPath:     config.CaddySubLog,
	}))
}

// ProvisionWatchdog writes the proxy watchdog systemd unit file.
func ProvisionWatchdog(ctx context.Context, proxyBin string) error {
	return provisionUnit(ctx, config.WatchdogService, renderUnit(unitSpec{
		Description: "Proxy Watchdog Service",
		After:       "network.target sing-box.service",
		ExecStart:   fmt.Sprintf("%s watchdog", proxyBin),
		RestartSec:  "30s",
		LogPath:     config.WatchdogLog,
	}))
}
