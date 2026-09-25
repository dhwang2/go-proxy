# go-proxy-cli

A non-interactive Go CLI for managing proxy protocols, users, routing and host networking on a Linux server, built on sing-box, Snell, ShadowTLS and Caddy.

Executable: `gproxy`. The project is named go-proxy-cli; the command, the runtime root `/etc/go-proxy` and the `version` output keep the shorter `go-proxy` spelling so existing installations keep working. Every command prints a readable result on a terminal and a stable JSON envelope with `--json`; no terminal UI is involved.

## Install

The installer downloads a published Linux amd64/arm64 release, verifies its checksum, installs `/usr/bin/gproxy` and initializes the runtime. Re-running it upgrades in place and moves a running watchdog onto the new binary. It also installs shell completion and enables it in every interactive root shell. Nothing runs until the whole script has arrived, so an interrupted `curl | bash` is harmless.

```bash
curl -fsSL https://raw.githubusercontent.com/dhwang2/go-proxy/main/app/install.sh | sudo bash
```

Set `VERSION=vX.Y.Z` to install a specific release, or `REPO` to install from another repository. `gproxy update` replaces the executable from the same place.

## Quick start

```bash
gproxy user add alice
gproxy protocol add                                   # lists every protocol with its options
gproxy protocol add vless --reality --user alice --port auto
gproxy protocol add tuic --user alice --port auto --domain proxy.example.com
gproxy sub alice                                      # client links and configuration
gproxy status                                         # the dashboard
```

A command that is missing something prints the complete command that would work, and exits 2. Fresh Reality and ShadowTLS installations select a random, TLS-verified handshake domain; certificate-based protocols need a domain you control.

## Commands

```text
gproxy status                                    dashboard: system, network, protocols, services, certificate
gproxy version

gproxy server status
gproxy server start|stop|restart <service>|--all

gproxy protocol list
gproxy protocol add <protocol> --user <name> --port <port|auto> [protocol options]
gproxy protocol remove <index|tag> [--user <name>] --confirm

gproxy user list
gproxy user add <name> [--all-protocols]
gproxy user rename <old> <new>
gproxy user remove <name> --confirm

gproxy route rule list [--user <name>]
gproxy route rule add    --user <name> --rules <indexes> --out <direct|chain tag>
gproxy route rule modify --user <name> --rules <indexes> --out <direct|chain tag>
gproxy route rule remove --user <name> --rules <indexes> --confirm
gproxy route rule remove [--user <name>] --all --confirm
gproxy route chain list
gproxy route chain add    <tag> --parameter <host>:<port>[:<username>:<password>] [--dns <resolver>]
gproxy route chain modify <tag> [--parameter <host>:<port>[:<username>:<password>]] [--dns <resolver>]
gproxy route chain remove <tag> --confirm
gproxy route direct list
gproxy route direct set <ipv4_only|ipv6_only|prefer_ipv4|prefer_ipv6|asis|auto>
gproxy route final list
gproxy route final set <direct|chain tag>
gproxy route sync-dns
gproxy route test --user <name> --domain <domain|ip>

gproxy sub [<user>] [--node <tag>] [--target <ip|host>] [--surge|--uri|--mihomo]

gproxy config view <sing-box|snell|shadow-tls> [--show-secrets]
gproxy config validate

gproxy core version|check|update [<component>|--all] [--version <v>]

gproxy network bbr status|enable
gproxy network firewall status|apply|release|add|remove [<port>/<tcp|udp|both>]
gproxy network fail2ban status|enable|disable

gproxy log <service> [--lines <n>] [--max-bytes <n>] [--follow]
gproxy cert status|ensure [--domain <domain>] [--email <address>]
gproxy update [--check] [--version <v>]
gproxy uninstall [--preview] [--confirm]
gproxy completion bash|zsh|fish|powershell
```

### Routing

Rules are numbered with the preset menu that `gproxy route rule add` prints on its own: `1`–`9` and `g`–`o` for services (OpenAI, Anthropic, Google, YouTube, Telegram, GitHub, Discord, …), `a` for international AI, `b` Netflix, `d` Disney+, `s` Spotify, `t` TikTok, `r` ads. The same numbers select rules in `route rule list`, `modify` and `remove`, several at a time.

```bash
gproxy route chain add res1 --parameter 198.51.100.7:1080:user:secret
gproxy route rule add --user alice --rules 1,2,a --out res1       # AI services through the chain
gproxy route rule modify --user alice --rules 2 --out direct
gproxy route test --user alice --domain chatgpt.com
gproxy route final set res1                                       # send everything else through res1 too
```

- `route rule add` only creates rules. A preset the user already has is left as it is and shown struck through as `(already added)`; change its outbound with `route rule modify`.
- `route test` walks the compiled rules in sing-box's order and matches remote rule sets from sing-box's own cache, so it answers without sending traffic or touching the network: the rule that takes the name (or the route final), the outbound, and whether its DNS lookup leaves the same way.
- A chain that rules still use cannot be removed; the refusal lists those rules and the commands that move them off it.

### Network

```bash
gproxy network firewall status
gproxy network firewall add 8443/tcp
gproxy network firewall remove 8443/both --confirm
gproxy network fail2ban status
```

- `network firewall status` says whether gproxy's nftables table is applied and in sync, and lists the ports gproxy keeps open. `apply` installs the table, which opens those ports and drops other inbound traffic; from then on every node, certificate or custom-port change keeps it in step. `release --confirm` removes only gproxy's table.
- `network fail2ban disable` stops fail2ban and lifts its bans; `enable` starts it with gproxy's SSH jail. `status` names the files that switch the SSH jail on and writes the banned addresses to `/etc/go-proxy/logs/fail2ban-banned.txt`.

## Output and automation

- Human output is coloured only on a terminal; pipes, files, `NO_COLOR`, `TERM=dumb` and `--no-color` get plain text.
- `--json` writes exactly one envelope, `{"ok", "changed", "data", "error"}`, and never contains escape sequences.
- Exit codes: `0` success (a repeated operation is a no-op with `changed: false`), `1` operation failure, `2` usage error, `130` cancelled.
- Destructive commands require `--confirm`. `--timeout` overrides an operation's deadline.

[app/CLI.md](app/CLI.md) is the complete reference: every command's contract, JSON fields, credentials handling and verification workflow.

## Development

```bash
cd app
make build VERSION=v0.3.2-dev
make test
./gproxy --help
```

Cross-build on a development or CI machine with `make release` (linux/amd64) or `make release-arm` (linux/arm64); do not compile on a memory-constrained deployment host. Tests live in `app/_tests` and run through the Go overlay generated by `scripts/generate-test-overlay.sh`; `make test` does both.

## Runtime and resources

The runtime root is `/etc/go-proxy`. Read-only commands work from local state and make no public-network calls, except `gproxy status`, which looks up the public address of an address family that is only NATed. Downloads are streamed, heavy mutations are serialized, and operations support cancellation. The resource target is a shared 2-vCPU, 1-GiB host.

Subscription exports contain connection credentials. Save them with restrictive permissions and keep them out of logs and public reports.
