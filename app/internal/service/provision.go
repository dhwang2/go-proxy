package service

import (
	"context"
	"fmt"

	"go-proxy/internal/config"
	"go-proxy/pkg/fileutil"
)

// provisionUnit writes a systemd unit file and reloads the daemon.
func provisionUnit(ctx context.Context, path, content string) error {
	if err := fileutil.AtomicWrite(path, []byte(content)); err != nil {
		return err
	}
	return DaemonReload(ctx)
}

// ProvisionSingBox writes the sing-box systemd unit file.
func ProvisionSingBox(ctx context.Context) error {
	unit := fmt.Sprintf(`[Unit]
Description=sing-box service
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target

[Service]
Type=simple
ExecStart=%s run -c %s
Restart=on-failure
RestartSec=10s
LimitNOFILE=infinity
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, config.SingBoxBin, config.SingBoxConfig, config.SingBoxLog, config.SingBoxLog)

	return provisionUnit(ctx, config.SingBoxService, unit)
}

// ProvisionSnell writes the snell systemd unit file.
func ProvisionSnell(ctx context.Context) error {
	unit := fmt.Sprintf(`[Unit]
Description=Snell v5 Proxy Service
After=network.target

[Service]
Type=simple
ExecStart=%s -c %s
Restart=on-failure
RestartSec=10s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, config.SnellBin, config.SnellConfigFile, config.SnellLog, config.SnellLog)

	return provisionUnit(ctx, config.SnellService, unit)
}

// ProvisionShadowTLS writes the shadow-tls systemd unit file.
func ProvisionShadowTLS(ctx context.Context, listenPort int, password, sni string, backendPort int) error {
	unit := fmt.Sprintf(`[Unit]
Description=Shadow-TLS v3 Service
After=network.target

[Service]
Type=simple
ExecStart=%s --v3 server --listen 0.0.0.0:%d --server 127.0.0.1:%d --tls %s --password %s
Restart=on-failure
RestartSec=10s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, config.ShadowTLSBin, listenPort, backendPort, sni, password,
		config.ShadowTLSLog, config.ShadowTLSLog)

	return provisionUnit(ctx, config.ShadowTLSService, unit)
}

// ProvisionCaddySub writes the caddy subscription server systemd unit file.
func ProvisionCaddySub(ctx context.Context) error {
	unit := fmt.Sprintf(`[Unit]
Description=Caddy Subscription Server
After=network.target

[Service]
Type=simple
Environment=XDG_DATA_HOME=/etc/go-proxy
Environment=XDG_CONFIG_HOME=/etc/go-proxy
ExecStart=%s run --config %s --adapter caddyfile
Restart=on-failure
RestartSec=10s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, config.CaddyBin, config.CaddyFile, config.CaddySubLog, config.CaddySubLog)

	return provisionUnit(ctx, config.CaddySubService, unit)
}

// ProvisionWatchdog writes the proxy watchdog systemd unit file.
func ProvisionWatchdog(ctx context.Context, proxyBin string) error {
	unit := fmt.Sprintf(`[Unit]
Description=Proxy Watchdog Service
After=network.target sing-box.service

[Service]
Type=simple
ExecStart=%s watchdog
Restart=on-failure
RestartSec=30s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`, proxyBin, config.WatchdogLog, config.WatchdogLog)

	return provisionUnit(ctx, config.WatchdogService, unit)
}
