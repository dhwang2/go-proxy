# go-proxy-cli — Agent Guide

Executable: `gproxy`. It runs without a TTY, menus, confirmation prompts or terminal control codes. Use `--json` for automation and capture stdout and stderr separately.

This branch builds a `v0.2.0-dev` candidate. The previously published stable installer may still install the older release until the CLI milestone is published. Do not self-update a development candidate to an unrelated stable binary during testing.

## Operating contract

- `gproxy --help` and `gproxy version` need no initialization or root. Run managed-state operations as root; over SSH, `sudo -n gproxy ...` fails promptly when elevation is unavailable.
- `gproxy init --json` prepares runtime configuration, binaries and service definitions. It does not create a user or proxy node. Repeat calls preserve configuration.
- Every finite operation supports `--json`; native subscription formats are mutually exclusive with it. Help remains human-readable even with `--json`.
- Exit **0** means the operation completed. A status query may complete while reporting `healthy:false`; check its data. Exit **2** means the request/arguments/confirmation are invalid. Exit **1** means an operational failure, timeout or partial application. Signals return **130** (SIGINT) or **143** (SIGTERM).
- Successful JSON has `ok`, `changed` and `data`. Failure JSON has `ok:false`, `changed`, `error.code`, `error.message`, optional `error.stage` and any available state in `data`.
- `changed:false` on a repeated operation is a valid no-op. Never infer success solely from exit 0 if the requested condition is a field in `data`.
- On `busy`, retry with bounded backoff. On `conflict`, reread state before retrying. On partial activation failure, inspect `data.pending`, configuration and service status; do not regenerate a node or credentials blindly.
- Destructive remove/delete/clear/uninstall commands require an explicit target and `--yes`. `--yes` does not enlarge the selected scope.
- A command group invoked without a subcommand is a usage error: exit **2** with `error.code` `invalid_argument` and a message naming the available subcommands. This applies to `cert`, `config`, `network`, `network bbr`, `network fail2ban`, `network firewall` and `routing chain`. Nothing is written to stdout in human mode, and under `--json` the usual one-value error envelope is emitted, so `gproxy network --json | jq` fails loudly rather than behind exit 0. Explicit `--help` is unaffected and still exits **0**, as does bare `gproxy`.
- Local status/list commands do not make public-network probes. Add `--probe` to status/network status when live public-address/connectivity information is needed; `not_checked` is not a failure or proof of no IPv6.
- `--timeout 30s` overrides the command deadline. Downloads/certificate operations report progress to stderr; cancellation terminates owned work. `log --follow` and `watchdog` are explicit streams and reject `--json`.
- If stdout breaks or output is cancelled, a partial document may exist. Discard it after a nonzero exit; there is no second JSON error appended to an already-started result.

Example success:

```json
{"ok":true,"changed":false,"data":{"user":"alice"}}
```

Example partial failure (inspect the actual returned fields):

```json
{"ok":false,"changed":true,"error":{"code":"service_activation_failed","stage":"activate","message":"service activation failed"},"data":{"applied":["configuration"],"pending":["sing-box"]}}
```

## Install and export a node

```bash
gproxy init --json
gproxy user add alice --json
gproxy protocol install vless --reality --user alice --port auto --json
gproxy protocol --json
gproxy sub alice --json
```

Use the returned `data.tag`, `data.port` and `data.sni` from installation. An automatic port is selected only when explicitly requested; repeat installation reuses an unambiguous matching node. If several nodes match, specify a numeric port.

For every fresh Reality node or ShadowTLS binding, the default handshake domain is randomly selected from a built-in candidate pool and checked for TLS 1.3, hostname and certificate validity. These are real domains, not invented hostnames. Repeating an existing installation preserves its selected SNI and credentials. Random choices may repeat. `www.apple.com` is not accepted.

The pool contains `www.kernel.org`, `www.freebsd.org`, `www.openbsd.org`, `www.rust-lang.org` and `www.postgresql.org`. Availability and protocol compatibility can change. If no candidate passes, installation fails explicitly. Supply `--sni <domain>` or `--shadow-tls-sni <domain>` to choose one; an explicit domain is checked and never silently replaced. A successful TLS probe is not a substitute for a real proxy traffic check.

Ordinary TLS protocols need a domain you control and a usable certificate; random handshake selection applies only to Reality/ShadowTLS:

```bash
gproxy protocol install vless --user alice --port 24443 --domain proxy.example.com --json
gproxy protocol install tuic --user alice --port 24444 --domain proxy.example.com --congestion cubic --json
gproxy protocol install anytls --user alice --port 24445 --domain proxy.example.com --json
gproxy protocol install ss --user alice --port auto --method 2022-blake3-aes-128-gcm --json
gproxy protocol install snell --user alice --port auto --ipv6 --json
```

SS defaults to `2022-blake3-aes-256-gcm`; TUIC defaults to `bbr`. Snell supports one owner and its IPv6 flag controls IPv6 egress. Supported listeners use dual stack when available.

Wrap SS or Snell during installation:

```bash
gproxy protocol install ss --user alice --port auto --shadow-tls --shadow-tls-port auto --json
```

A matching existing binding is reused. Conflicting settings fail instead of replacing credentials or moving ports.

## Subscription output and secrets

```bash
gproxy sub
gproxy sub alice
gproxy sub alice --node vless_reality_24443 --uri
gproxy sub alice --node vless_reality_24443 --sing-box
gproxy sub alice --node snell-v6 --surge
gproxy sub alice --json
```

- Default output shows usable import content and node labels. `--json` returns `data.nodes`, each node's available `formats` and credential-bearing `content`.
- `--surge`, `--sing-box`, `--uri` and `--json` are mutually exclusive. The spelling is **sing-box**, including the flag.
- Native sing-box export is one JSON document with an `outbounds` array, not a complete local client configuration. Native Surge/URI exports contain import content only.
- Unsupported selected formats fail before writing stdout. Narrow the selection with `--node`: VLESS has no Surge renderer, Snell is Surge-only, and wrapped SS/Snell use Surge rather than a misleading direct URI.
- `--target <IP>` avoids DNS/HTTP discovery. A hostname target and automatic dual-stack selection use bounded resolution. If automatic detection cannot find a usable target, supply one explicitly.
- Subscription exports and `config view --show-secrets` contain secrets. Use private files; never paste them into reports or public release notes. Ordinary inspection and errors are redacted.

```bash
umask 077
gproxy sub alice --node vless_reality_24443 --sing-box > client-outbounds.json
```

There is no HTTP subscription publishing server in this CLI; Caddy is used for certificates.

## User and node lifecycle

```bash
gproxy user --json
gproxy user add bob --all-protocols --json
gproxy user rename bob charlie --json
gproxy protocol remove vless_reality_24443 --user charlie --yes --json
gproxy user delete charlie --yes --json
gproxy protocol remove vless_reality_24443 --yes --json
```

Plain user creation only registers a name. `--all-protocols` explicitly enrolls in existing sing-box nodes without transferring Snell ownership. Removing the last membership removes its node and dependent ShadowTLS binding; unrelated users/nodes remain intact. Use returned tags rather than menu indexes or guessed names.

## Inspect, services and certificates

```bash
gproxy status --json
gproxy status --probe --timeout 10s --json
gproxy config view sing-box --json
gproxy config view snell --json
gproxy config view shadow-tls --json
gproxy config validate --json
gproxy start sing-box --json
gproxy restart --all --json
gproxy stop sing-box --json
gproxy cert status --json
gproxy cert ensure --domain proxy.example.com --email admin@example.com --json
gproxy log sing-box --lines 100 --json
gproxy log proxy-watchdog --follow
gproxy log sing-box --lines 500 --max-bytes 65536
```

`--lines` bounds how many lines are returned. `--max-bytes` is a hard ceiling, not a truncation point: when the selected log exceeds it the command fails with `log output exceeds byte limit` and writes no partial log, so a large or fast-growing journal cannot produce unbounded output. Raise `--max-bytes` or lower `--lines` to fit. `--follow` streams until cancelled and cannot be combined with `--json`.

Managed service selectors include `sing-box`, `snell-v6`, `shadow-tls`, `caddy-sub`, `proxy-watchdog` and known dynamic ShadowTLS unit names. Service actions require a selector or `--all`. Explicit stop is remembered; automatic recovery does not undo it. Use explicit start/restart to resume. Configuration changes to an intentionally stopped service report that pending activation.

## Routing and network

```bash
gproxy routing presets --json
gproxy routing list alice --json
gproxy routing set alice --preset openai --outbound direct --json
gproxy routing modify alice --rules 1 --outbound direct --json
gproxy routing remove alice --rules 1 --yes --json
gproxy routing direct --strategy prefer_ipv6 --json
gproxy routing sync-dns --json
gproxy routing test alice example.com --json
gproxy routing clear alice --yes --json
gproxy routing chain list --json
gproxy routing chain add upstream --host proxy.example.net --port 1080 --credentials-file auth.json --json
gproxy routing chain remove upstream --yes --json
gproxy network status --probe --json
gproxy network bbr status --json
gproxy network bbr enable --json
gproxy network firewall status --json
gproxy network firewall add 9443 --transport both --json
gproxy network firewall apply --json
gproxy network firewall remove 9443 --transport both --yes --json
gproxy network firewall clear --yes --json
gproxy network fail2ban status --json
gproxy network fail2ban enable --json
gproxy network fail2ban disable --json
```

Rule indexes are 1-based and come from a fresh `routing list`; comma-separated indexes/presets support batch changes. Direct strategies are `ipv4_only`, `ipv6_only`, `prefer_ipv4`, `prefer_ipv6`, `asis`. Route tests explain local rules, and unresolved remote rule-set contents are reported honestly; they do not demonstrate network traffic.

SOCKS5 `auth.json` is a JSON object with `username` and `password`; both are required together. Use `--credentials-file -` to read it explicitly from stdin. Omit the flag for unauthenticated SOCKS5. Do not put credentials in ordinary flags. Firewall status includes current/desired/planned changes; applying rules retains DHCPv6/SSH transport requirements.

## Updates and removal

```bash
gproxy core --json
gproxy core check --timeout 30s --json
gproxy core update snell --json
gproxy core update sing-box --version 1.13.11 --json
gproxy update --check --timeout 30s --json
gproxy uninstall --preview --json
```

Core selectors are `sing-box`, `snell`, `shadow-tls`, `caddy`; `core update --all` updates installed cores sequentially. Snell currently uses the verified `6.0.0rc2` archive. Update commands validate integrity/version and replace binaries atomically; checks do not install anything. Unversioned self-update never downgrades a newer/development build to an older stable release. An explicit `--version` deliberately selects that release.

Only run `gproxy update` or `gproxy uninstall --yes` when replacement/removal is actually intended. Uninstall removes owned configuration, units, binaries and firewall state, not unrelated system journals or services. Runtime coordination files under `/run/lock/go-proxy` can remain until reboot; init installs a tmpfiles rule so read-only commands also work after reboot.

## Agent execution checklist

1. Read `version --json`, command help, and the relevant current-state query.
2. Build an explicit request with the correct user/tag/port and flags; never simulate keyboard input.
3. Capture stdout, stderr and exit status independently. Parse JSON only when requested and the output is complete.
4. Inspect `ok`, `changed`, error stage and actual postconditions. Validate configuration, required service/listener state and compatible client traffic for networking changes.
5. Retry only after classifying `busy`, conflict or pending activation. Preserve credentials and unrelated resources.
6. Keep exports private, terminate follow/test-client processes and remove only test-owned artifacts.
