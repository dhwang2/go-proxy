package config

import "path/filepath"

const (
	// WorkDir is the runtime root for go-proxy.
	WorkDir = "/etc/go-proxy"

	// BinDir holds managed binaries (sing-box, snell-server, shadow-tls, caddy).
	BinDir = WorkDir + "/bin"

	// ConfDir holds generated configuration files.
	ConfDir = WorkDir + "/conf"

	// LogDir holds service and script logs.
	LogDir = WorkDir + "/logs"

	// CaddyCertDir is the root certificate storage directory used by caddy-sub
	// (matches XDG_DATA_HOME=/etc/go-proxy so caddy writes to /etc/go-proxy/caddy/certificates/).
	CaddyCertDir = WorkDir + "/caddy/certificates"
)

// Config file paths.
var (
	SingBoxConfig    = filepath.Join(ConfDir, "sing-box.json")
	UserMetaFile     = filepath.Join(WorkDir, "user-management.json")
	UserRouteFile    = filepath.Join(WorkDir, "user-route-rules.json")
	UserTemplateFile = filepath.Join(WorkDir, "user-route-templates.json")
	SnellConfigFile  = filepath.Join(WorkDir, "snell-v5.conf")
	SubscriptionFile = filepath.Join(WorkDir, "subscription.txt")
	CaddyFile        = filepath.Join(WorkDir, "Caddyfile")
	DomainFile       = filepath.Join(WorkDir, ".domain")
)

// Binary paths.
var (
	SingBoxBin   = filepath.Join(BinDir, "sing-box")
	SnellBin     = filepath.Join(BinDir, "snell-server")
	ShadowTLSBin = filepath.Join(BinDir, "shadow-tls")
	CaddyBin     = filepath.Join(BinDir, "caddy")
)

// Systemd unit file paths.
const (
	SingBoxService   = "/etc/systemd/system/sing-box.service"
	SnellService     = "/etc/systemd/system/snell-v5.service"
	ShadowTLSService = "/etc/systemd/system/shadow-tls.service"
	CaddySubService  = "/etc/systemd/system/caddy-sub.service"
	WatchdogService  = "/etc/systemd/system/proxy-watchdog.service"
)

// Log file paths.
var (
	SingBoxLog   = filepath.Join(LogDir, "sing-box.service.log")
	SnellLog     = filepath.Join(LogDir, "snell-v5.service.log")
	ShadowTLSLog = filepath.Join(LogDir, "shadow-tls.service.log")
	CaddySubLog  = filepath.Join(LogDir, "caddy-sub.service.log")
	ScriptLog    = filepath.Join(LogDir, "proxy-script.log")
	WatchdogLog  = filepath.Join(LogDir, "proxy-watchdog.log")
)
