package config

import "path/filepath"

const (
	// WorkDir is the runtime root for go-proxy.
	WorkDir       = "/etc/go-proxy"
	LockDir       = "/run/lock/go-proxy"
	LockSetupFile = "/etc/tmpfiles.d/go-proxy.conf"

	// BashCompletionPath and ZshCompletionPath are where the installer writes
	// the generated completion scripts. They are owned resources: uninstall
	// removes exactly these two paths and nothing else under those directories.
	BashCompletionPath = "/usr/share/bash-completion/completions/gproxy"
	ZshCompletionPath  = "/usr/share/zsh/site-functions/_gproxy"

	// SystemBashrc is where Debian and Ubuntu configure every interactive
	// bash, and ship bash-completion's loader commented out: completion then
	// works only in login shells (sudo -i), not in sudo su or sudo -s. The
	// installer adds one block between these markers to load it everywhere;
	// uninstall removes exactly that block.
	SystemBashrc              = "/etc/bash.bashrc"
	BashrcCompletionBeginMark = "# >>> go-proxy: load bash completion in every interactive shell >>>"
	BashrcCompletionEndMark   = "# <<< go-proxy <<<"

	// The runtime root is laid out by what a file is, not by who wrote it:
	// bin/ is fetched, conf/ is generated configuration a core reads, data/ is
	// this program's own state, cache/ is regenerable, logs/ is output. A file
	// loose in the root belonged to one of these and said which by its name.
	//
	// caddy/ is the exception and stays where it is: caddy writes its
	// certificate store under XDG_DATA_HOME, which is the runtime root, and
	// moving it would abandon issued certificates that are rate-limited to
	// reissue.

	// BinDir holds managed binaries (sing-box, snell-server, shadow-tls, caddy).
	BinDir = WorkDir + "/bin"

	// ConfDir holds generated configuration files, each read by a core.
	ConfDir = WorkDir + "/conf"

	// DataDir holds this program's own persistent state.
	DataDir = WorkDir + "/data"

	// CacheDir holds regenerable data. Removing it costs a rebuild, not state.
	CacheDir = WorkDir + "/cache"

	// LogDir holds service logs.
	LogDir = WorkDir + "/logs"

	// CaddyCertDir is the root certificate storage directory used by caddy-sub
	// (matches XDG_DATA_HOME=/etc/go-proxy so caddy writes to /etc/go-proxy/caddy/certificates/).
	CaddyCertDir = WorkDir + "/caddy/certificates"
)

// Configuration a core reads.
var (
	SingBoxConfig   = filepath.Join(ConfDir, "sing-box.json")
	SnellConfigFile = filepath.Join(ConfDir, "snell-v6.conf")
	CaddyFile       = filepath.Join(ConfDir, "Caddyfile")
)

// State this program owns. Losing any of it loses user credentials or the
// routing that depends on them.
var (
	UserMetaFile       = filepath.Join(DataDir, "user-management.json")
	UserRouteFile      = filepath.Join(DataDir, "user-route-rules.json")
	UserTemplateFile   = filepath.Join(DataDir, "user-route-templates.json")
	FirewallConfigFile = filepath.Join(DataDir, "firewall-ports.json")
	DomainFile         = filepath.Join(DataDir, ".domain")
)

// Regenerable. SingBoxCache is rebuilt by sing-box from its rule sets.
var SingBoxCache = filepath.Join(CacheDir, "cache.db")

// Fail2BanBannedFile is the address list `network fail2ban` writes on every
// report, one per line, so the terminal shows a path instead of hundreds of
// addresses.
var Fail2BanBannedFile = filepath.Join(LogDir, "fail2ban-banned.txt")

// Binary paths.
var (
	SingBoxBin   = filepath.Join(BinDir, "sing-box")
	SnellBin     = filepath.Join(BinDir, "snell-server")
	ShadowTLSBin = filepath.Join(BinDir, "shadow-tls")
	CaddyBin     = filepath.Join(BinDir, "caddy")
)

// Systemd unit file paths.
const (
	SingBoxService  = "/etc/systemd/system/sing-box.service"
	SnellService    = "/etc/systemd/system/snell-v6.service"
	CaddySubService = "/etc/systemd/system/caddy-sub.service"
	WatchdogService = "/etc/systemd/system/proxy-watchdog.service"
)

// Log file paths.
var (
	SingBoxLog   = filepath.Join(LogDir, "sing-box.service.log")
	SnellLog     = filepath.Join(LogDir, "snell-v6.service.log")
	ShadowTLSLog = filepath.Join(LogDir, "shadow-tls.service.log")
	CaddySubLog  = filepath.Join(LogDir, "caddy-sub.service.log")
	WatchdogLog  = filepath.Join(LogDir, "proxy-watchdog.log")
)
