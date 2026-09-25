# go-proxy-cli — Agent Guide

Executable: `gproxy`. It needs no TTY and has no menus or confirmation prompts. Nothing it writes to stdout ever carries a terminal control code, and neither does stderr unless stderr is itself a terminal, where a step that takes minutes animates in place. Use `--json` for automation and capture stdout and stderr separately.

This guide describes v0.3.1. Since v0.2.0, service verbs moved under `server`, `protocol install` became `protocol add`, and `routing` became `route` with rules and chains as their own groups. One object is removed with one verb and listed with one verb: `user remove`, `route rule list`, `route chain list`. Rules are selected by preset index (`--rules 1,3,a`), a chain endpoint is one `--parameter`, and a custom firewall port is written `<port>/<tcp|udp|both>`. See the migration table in [the redesign](../docs/plans/v0.3/cli-ux-redesign.md). Do not self-update a development build to an unrelated stable binary during testing.

## Operating contract

- `gproxy --help` and `gproxy version` need no initialization or root. Run managed-state operations as root; over SSH, `sudo -n gproxy ...` fails promptly when elevation is unavailable.
- `gproxy init --json` prepares runtime configuration, binaries and service definitions. It does not create a user or proxy node. Repeat calls preserve configuration. It is hidden from the command list because `install.sh` runs it as its last step; invoke it directly only when building from source.
- `gproxy watchdog` is hidden for the same reason: it is the `ExecStart` of `proxy-watchdog.service`, not a command to run by hand. Run directly it occupies the terminal until interrupted. Read what it did with `gproxy log proxy-watchdog`, and control it with `gproxy server start|stop|restart proxy-watchdog`.
- Every finite operation supports `--json`; native subscription formats are mutually exclusive with it. Help remains human-readable even with `--json`.
- Exit **0** means the operation completed. A status query may complete while reporting `healthy:false`; check its data. Exit **2** means the request/arguments/confirmation are invalid. Exit **1** means an operational failure, timeout or partial application. Signals return **130** (SIGINT) or **143** (SIGTERM).
- A name no object has is an invalid argument, not an operational failure: `--user` naming an unknown user, or `--out` naming an unknown outbound, is `invalid_argument` at exit **2** in every command that takes one.
- Successful JSON has `ok`, `changed` and `data`. Failure JSON has `ok:false`, `changed`, `error.code`, `error.message`, optional `error.stage` and any available state in `data`.
- `changed:false` on a repeated operation is a valid no-op. Never infer success solely from exit 0 if the requested condition is a field in `data`. Removing something that is not there is such a no-op: `user remove`, `protocol remove`, `route chain remove` and `network firewall release` all exit **0** and carry `data.removed`, which is `false` when the named target was already absent.
- On `busy`, retry with bounded backoff. On `conflict`, reread state before retrying. On partial activation failure, inspect `data.pending`, configuration and service status; do not regenerate a node or credentials blindly.
- Destructive remove/clear/uninstall commands require an explicit target and `--confirm`. `--confirm` does not enlarge the selected scope. It is checked with the argument checks, so a missing confirmation, a rejected name and a missing argument all fail before the command reports having started anything.
- A command group invoked without a subcommand is a usage error: exit **2** with `error.code` `invalid_argument` and a message naming the available subcommands. This applies to every group: `cert`, `config`, `core`, `network`, `network bbr`, `network fail2ban`, `network firewall`, `protocol`, `route`, `route chain`, `route rule`, `server` and `user`. No group has a default action. Nothing is written to stdout in human mode, and under `--json` the usual one-value error envelope is emitted, so `gproxy network --json | jq` fails loudly rather than behind exit 0. Explicit `--help` is unaffected and still exits **0**, as does bare `gproxy`.
- Local status/list commands do not make public-network probes, with one exception: for an address family whose interfaces carry only private addresses (the host is behind NAT), `gproxy status` asks a public endpoint for the address clients see, within its deadline, and shows it in place of the private one. `status` takes no options besides the global ones. `not_checked` is not a failure or proof of no IPv6.
- `--timeout 30s` overrides the command deadline. Downloads/certificate operations report progress to stderr; cancellation terminates owned work. `log --follow` and `watchdog` are explicit streams and reject `--json`.
- Progress is one line per step, and only for steps that do real work (a download, a certificate, a firewall change); there is no opening line repeating the command, and a fast change such as a routing rule prints none. On a terminal the step in progress is rewritten in place with the time it has taken, and is left behind as an ordinary line when it ends, so the transcript matches what a pipe receives; anywhere else the same steps are written once, with a five-second `operation in progress` while one of them is long. An animated step is finished before any result or error is written, and the animation never reaches stdout.
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
gproxy protocol add vless --reality --user alice --port auto --json
gproxy user list --json
gproxy sub alice --json
```

Use the returned `data.tag`, `data.port` and `data.sni` from installation. An automatic port is selected only when explicitly requested; repeat installation reuses an unambiguous matching node. If several nodes match, specify a numeric port.

For every fresh Reality node or ShadowTLS binding, the default handshake domain is randomly selected from a built-in candidate pool and checked for TLS 1.3, hostname and certificate validity. These are real domains, not invented hostnames. Repeating an existing installation preserves its selected SNI and credentials. Random choices may repeat. `www.apple.com` is not accepted.

The pool contains `www.kernel.org`, `www.freebsd.org`, `www.openbsd.org`, `www.rust-lang.org` and `www.postgresql.org`. Availability and protocol compatibility can change. If no candidate passes, installation fails explicitly. Supply `--sni <domain>` or `--shadow-tls-sni <domain>` to choose one; an explicit domain is checked and never silently replaced. A successful TLS probe is not a substitute for a real proxy traffic check.

Ordinary TLS protocols need a domain you control and a usable certificate; random handshake selection applies only to Reality/ShadowTLS:

```bash
gproxy protocol add vless --user alice --port 24443 --domain proxy.example.com --json
gproxy protocol add tuic --user alice --port 24444 --domain proxy.example.com --congestion cubic --json
gproxy protocol add anytls --user alice --port 24445 --domain proxy.example.com --json
gproxy protocol add snell --user alice --port auto --ipv6 --json
```

TUIC defaults to `bbr`. Snell supports one owner and its IPv6 flag controls IPv6 egress. Supported listeners use dual stack when available.

Wrap Snell during installation:

```bash
gproxy protocol add snell --user alice --port auto --shadow-tls --shadow-tls-port auto --json
```

A matching existing binding is reused. Conflicting settings fail instead of replacing credentials or moving ports.

Certificate issuance waits on Caddy and reads Caddy's log while it waits. A refusal that names the time it lifts — the certificate authority's weekly limit for that exact name, most often — ends the wait immediately and is reported as the authority worded it, including when it can be retried, rather than spending the five-minute deadline and blaming DNS. A failure the authority will retry keeps the wait, and if that wait does run out the reason is still the one Caddy recorded.

`--domain` is required to **create** a node that needs a certificate, and not to join one that exists: an installed node was issued for a domain, and enrolling another user into it reuses that domain and that certificate. Naming a different one is still a conflict rather than a silent reissue. Because whether a node exists is a question about state, that requirement is checked against a read-only snapshot before the operation starts, so it still fails as an argument error and leaves nothing behind.

## Subscription output and secrets

```bash
gproxy sub
gproxy sub alice
gproxy sub alice --node vless_reality_24443 --uri
gproxy sub alice --node vless_reality_24443 --mihomo
gproxy sub alice --node snell-v6 --surge
gproxy sub alice --mihomo
gproxy sub alice --json
```

- A link is named `<host>-<protocol>[-<port>]-<family>-<user>`. The port appears only where it is needed to tell two links apart, which is when one user has more than one node of the same protocol: names key the proxy list in every client that reads these, so two links sharing one would silently replace each other.
- Default output carries every format each node supports, grouped for reading: a `[format]` heading, the user under it, and that user's links below — each heading printed once, not above every line. Within a user the address families sit together, so one block can be taken at once. The links themselves carry no colour: they are what gets copied. An explicit format still streams the artifact itself, unchanged. `--json` returns `data.nodes`, each node's available `formats` and credential-bearing `content`.
- `--surge`, `--uri`, `--mihomo` and `--json` are mutually exclusive. There is no sing-box client export: it was withdrawn in v0.3.1.
- `--mihomo` writes a `proxies:` document, one flow-style YAML mapping per node per address family, ready to load. The default view prints the same wrapper around its `[mihomo]` section — the `proxies:` key, the sequence indent, and each user's name as a YAML comment — so that section loads as written too. Surge and URI exports contain import content only.
- A mapping drops the space after each comma but keeps the one after each colon. Dropping the second still parses — into a mapping whose keys are the whole `name:"value"` string and whose values are all null — so the configuration would be silently wrong rather than rejected.
- A selected format exports the nodes that support it and names the rest on stderr: a user whose nodes do not all share one format still gets the ones that do. Selecting a single node with `--node` is an explicit pair, so a node with no export in that format still fails.
- Format coverage: VLESS has no Surge renderer; Snell is Surge-only, which also covers Snell behind ShadowTLS rather than offering a misleading direct URI. Snell has no mihomo entry because mihomo implements Snell v1–v5 while this project deploys snell-server v6, whose wire format derives a per-deployment profile from the PSK — a v5 entry would load and then fail to connect.
- `--target <IP>` avoids DNS/HTTP discovery. A hostname target and automatic dual-stack selection use bounded resolution. If automatic detection cannot find a usable target, supply one explicitly.
- Subscription exports and `config view --show-secrets` contain secrets. Use private files; never paste them into reports or public release notes. Ordinary inspection and errors are redacted.

```bash
umask 077
gproxy sub alice --node vless_reality_24443 --mihomo > client-proxies.yaml
```

There is no HTTP subscription publishing server in this CLI; Caddy is used for certificates.

## User and node lifecycle

```bash
gproxy user list --json
gproxy user add bob --all-protocols --json
gproxy user rename bob charlie --json
gproxy protocol remove vless_reality_24443 --user charlie --confirm --json
gproxy user remove charlie --confirm --json
gproxy protocol remove vless_reality_24443 --confirm --json
```

Plain user creation only registers a name. `--all-protocols` explicitly enrolls in existing sing-box nodes without transferring Snell ownership. Removing the last membership removes its node and dependent ShadowTLS binding; unrelated users/nodes remain intact. `user remove` reports what it cascaded into — `data.nodes_removed` for the nodes that were the user's alone and `data.rules_removed` for its routing rules — because a deletion that takes several objects with it should say how many. On a terminal each user command answers in one line: `alice (added)` or `(already exists)`, `bob -> charlie (changed)`, and `charlie (removed; 1 node, 3 rules)` with the name struck through, or `(not found)`; `user add` carries `data.added`. Use returned tags rather than menu indexes or guessed names.

## Inspect, services and certificates

```bash
gproxy status --json
gproxy server status --json
gproxy config view sing-box --json
gproxy config view snell --json
gproxy config view shadow-tls --json
gproxy config validate --json
gproxy server start sing-box --json
gproxy server restart --all --json
gproxy server stop sing-box --json
gproxy cert status --json
gproxy cert ensure --json
gproxy cert ensure --domain proxy.example.com --email admin@example.com --json
gproxy log sing-box --lines 100 --json
gproxy log proxy-watchdog --follow
gproxy log sing-box --lines 500 --max-bytes 65536
```

`cert status` and `cert ensure` answer in one line, `domain: proxy.example.com (expires in 88 days)` (or `(not issued)`, `(expired)`). `cert ensure` without `--domain` works on the configured domain: an existing certificate is reported as it stands, since Caddy renews it; a missing one is issued, which spends one of Let's Encrypt's five issuances per domain per week. With `--domain`, it issues for that domain. With neither a flag nor a configured domain it prints `gproxy cert ensure --domain <domain> [--email <address>]` and exits 2.

`gproxy log` takes a service; without one it answers with the signature and one command that can be run as written, rather than guessing which service was meant. The selectors are in shell completion and in the `--json` envelope. The rendering is the log itself — the service and its source, then the lines verbatim, unnumbered and unindented, so a line survives being copied or piped to `grep`. Lines naming `ERROR`, `FATAL`, `FAILED` or `PANIC` are red and `WARN` lines amber; routine lines are left plain so the others stand out. A log that colours its own output has those escape sequences removed, under `--follow` as well.

`--lines` bounds how many lines are returned. `--max-bytes` is a hard ceiling, not a truncation point: when the selected log exceeds it the command fails with `log output exceeds byte limit` and writes no partial log, so a large or fast-growing journal cannot produce unbounded output. Raise `--max-bytes` or lower `--lines` to fit. `--follow` streams until cancelled and cannot be combined with `--json`.

`gproxy status` is the whole-host dashboard, five numbered rows: the distribution, the host's IPv4 and IPv6 addresses, what each user has, the managed services with their state, and the certificate with its remaining validity. The service row carries the state as colour where there is colour, and as a word — `sing-box (running)`, `snell (stopped)`, `caddy (absent)` — where there is not, so a piped or `--no-color` reading still answers which services are running. Behind NAT the network row shows the public address in place of the private interface address, and marks the private one `(internal)` only when that lookup fails; a family with a public interface address is shown as it is, with no lookup. Each row has its own colour on a terminal: system blue, network purple, protocol yellow, services by state, domain amber. `gproxy server status` is the service list alone, one per line with the enablement detail. Managed service selectors include `sing-box`, `snell-v6`, `shadow-tls`, `caddy-sub`, `proxy-watchdog` and known dynamic ShadowTLS unit names; the dashboard's short names `snell`, `caddy` and `watchdog` select the same units. `log shadow-tls` reads the one ShadowTLS unit's log, and asks for a unit name when there are several. Service actions require a selector or `--all`. Explicit stop is remembered; automatic recovery does not undo it. Use explicit start/restart to resume. Configuration changes to an intentionally stopped service report that pending activation.

## Routing and network

```bash
gproxy route rule list --user alice --json
gproxy route rule add --user alice --rules 1,3,a --out direct --json
gproxy route rule modify --user alice --rules 1 --out direct --json
gproxy route rule remove --user alice --rules 1 --confirm --json
gproxy route rule remove --user alice --all --confirm --json
gproxy route direct list --json
gproxy route direct set prefer_ipv6 --json
gproxy route final list --json
gproxy route final set res1 --json
gproxy route sync-dns --json
gproxy route test --user alice --domain example.com --json
gproxy route chain list --json
gproxy route chain add upstream --parameter 198.51.100.7:1080:alice:secret --json
gproxy route chain modify upstream --parameter 198.51.100.7:1081:alice:secret --json
gproxy route chain remove upstream --confirm --json
gproxy network bbr status --json
gproxy network bbr enable --json
gproxy network firewall status --json
gproxy network firewall add 9443/both --json
gproxy network firewall apply --json
gproxy network firewall remove 9443/both --confirm --json
gproxy network firewall release --confirm --json
gproxy network fail2ban status --json
gproxy network fail2ban enable --json
gproxy network fail2ban disable --json
```

`route rule list` groups rules under the user that owns them and numbers each rule with its preset's menu index, which is the numbering `--rules` takes; several numbers at once support batch changes. Each rule line names the outbound it selects and, when that outbound is a chain, the server behind it, so one line answers where the traffic goes. `route chain list` prints each chain as a rule names it, `res1 -> 198.51.100.7:1080 (ipv4_only/https dns.google 8.8.8.8:443)`: the tag, its server, and in brackets the address family its lookups use and the resolver they go to; `final` follows a chain that is the route final. Whether it authenticates and the users whose rules select it stay in `--json` (`authenticated`, `users`). `route direct list|set` and `route sync-dns` answer `direct strategy: prefer_ipv4`. `route chain remove` refuses a chain that is still in use, with exit **1** and `error.code` `conflict`, and removes nothing. `data.rules` lists every rule that still selects the chain, and `data.route_final` says whether it is also the route final. The human rendering prints those rules the way `route rule list` does, then each user's `route rule modify … --out direct` and `route rule remove … --confirm` with the selectors filled in, and `route final set direct` when the chain is also the route final. Direct strategies are `ipv4_only`, `ipv6_only`, `prefer_ipv4`, `prefer_ipv6`, `asis` and `auto`. `auto` reads the families this host has and stores the concrete strategy that matches: only IPv4 gives `ipv4_only`, only IPv6 gives `ipv6_only`, and a dual-stack host gives `prefer_ipv4`. The word `auto` is never stored. `route sync-dns` recompiles the route and DNS rules from the stored user rules without changing any of them. Every other routing operation already recompiles as part of its own change, so this is for picking up a change in how rules compile — a new build deciding a chain's resolver families differently, say — without inventing a state change to trigger it. It reports `changed` only when the recompiled configuration actually differs from the one on disk.

`route test --user <name> --domain <domain|ip>` answers where one connection would leave, without sending it. It walks the compiled rules in `sing-box.json` in the order sing-box does and stops at the first that takes the connection, falling through to the route final. Remote rule sets are read out of sing-box's own cache (`experimental.cache_file`) and matched with `sing-box rule-set match`, two at a time, so the answer uses exactly the data the running server has and needs no network. A domain is evaluated as the name a client asked for: geoip sets, `ip_cidr` and private ranges decide only an IP. The one line names the target, the user's rule as `route rule list` numbers it with what matched in brackets, or `final`, then the outbound and whether its resolver leaves the same way — `dns ok`, or `dns via <outbound>` in red, a lookup leaving by a different address than its traffic. A set the cache does not hold yet is listed under the line, and the decision holds only if that set does not contain the target. `data.evaluation.decision` carries `rule` (the position in `route.rules`, `-1` for the final), `match_by`, `value`, `outbound`, `dns_server` and `dns_via`; `data.rule` is the user's rule and `data.address` the chain's server. It does not prove the chain carries traffic.

`route rule add` without `--user`, `--rules` or `--out` prints one runnable example and the preset menu, flush left, in shell-proxy's numbering: `1`-`9` (OpenAI/ChatGPT, Anthropic/Claude, Google, YouTube, Telegram, Twitter/X, WhatsApp, Facebook, GitHub), then `g`-`o`, `a`, `b`, `d`, `e`, `s`, `t`, `r`. `--rules` takes those indexes, several at once separated by commas (`--rules 1,3,5,9,a,b`); a preset name is refused, so one numbering is used everywhere; letters are case-insensitive, an index given twice is added once, and an unknown one is refused before anything runs. `add` only creates: a preset the user already has a rule for keeps its rule and outbound, and the listing shows it struck through and marked `(already added)`; change where it goes with `route rule modify`. The others in the same call are added, and a call that adds nothing reports no change. `route rule list` numbers each rule with its preset's menu index and lists them in menu order, and `route rule modify` and `route rule remove` take those numbers, several at once (`--rules 1,2,3,6,j,a`); a rule no preset made is numbered `c1`, `c2` and so on. A number the user has no rule for refuses the whole change. `route rule add`, `modify` and `remove` answer with the user's rules as they now stand, in the `route rule list` layout; rules a remove took out keep their place, struck through on a terminal and marked `(removed)` elsewhere. `--json` keeps its fields and adds `rules`, and `removed` for a remove. `route chain add` without a tag or `--parameter` explains the argument and the `--parameter` format. `route rule remove --all` replaces the old `routing clear`. `--out` takes `direct` or a chain tag, never a node tag.

There is no `network status`: the addresses it listed are the dashboard's network row, which looks up a NATed family's public address itself, and its ports are `network firewall status`. `network firewall status` opens with one line: `firewall not applied` (with whether nftables is installed), `firewall applied in sync`, or `firewall applied N changes pending`. Unapplied or in sync, the ports gproxy keeps open follow, one per line as `port/transport` and what asked for it. With drift, only the changes `apply` would make are listed, marked `+` and `-`. The last line is `gproxy network firewall apply` when there is a gap to close; unapplied, it adds that applying opens those ports and drops other inbound, because the managed table ends in a drop rule. The nftables ruleset itself stays in the `--json` payload. Once the table is applied, every node, certificate or custom-port change reconciles it, so `apply` is needed only to take the firewall over or to repair drift. A custom port is one argument, `<port>/<tcp|udp|both>` (`add 8443/tcp`, `remove 8443/both --confirm`); the transport stays required. `add` and `remove` print one line per transport, `8443/tcp  custom` with `(added)`, `(already added)` or `(not found)` beside it, a removed port struck through, and `(added; firewall not applied)` when the port is saved but no table is in place; `data.changes` lists `port`, `transport` and `result`. `release` prints `firewall  removed` or `firewall  not applied`. `network fail2ban status` reports the service, the jail with what switches it on in brackets — `gproxy` for gproxy's own jail file and the other files under `/etc/fail2ban` whose `[sshd]` section enables it, read in fail2ban's order (`data.managed`, `data.jail_sources`) — the thresholds that decide a ban and the counts. `disable` turns fail2ban off: it removes gproxy's jail file and runs `systemctl disable --now fail2ban`, so every jail stops, whoever configured it, and the bans they held are lifted; it answers `fail2ban  stopped (service disabled; N bans lifted)`, or `already stopped` with no progress line when there was nothing to stop. A stopped fail2ban's status is two rows, `fail2ban stopped` and `ssh jail off`. `enable` writes gproxy's jail, `/etc/fail2ban/jail.d/go-proxy-sshd.local`, and starts and enables the service — reloading it only if it was already running, since a service just started reads the jail as it starts and does not answer on its socket until it is up — then waits up to 15 seconds for the ssh jail to answer, answering `running` with `(started with gproxy's ssh jail)`, `(gproxy ssh jail added)` or `(gproxy ssh jail already in place)`. `data.change`, `data.bans_lifted` and `data.starts_at_boot` carry the same. `uninstall` still removes only gproxy's jail file and leaves fail2ban running. The addresses banned now are written to `/etc/go-proxy/logs/fail2ban-banned.txt`, one per line, rewritten whole on every status, enable or disable and emptied when nothing is banned; the `banned ip` row is that path, and `--json` carries both the full `banned_ips` list and `banned_file`. If the file cannot be written, the row falls back to five addresses and a count of the rest. This file is the one thing a status query writes.

`route final set <direct|chain tag>` chooses where connections no rule claims leave. `direct` is the default; a chain sends the whole server through it, shell-proxy's global mode, and moves `dns.final` to that chain's resolver so names are resolved from where they are dialled. Direct rules keep resolving through `route.default_domain_resolver`, and a chain that is the final cannot be removed until `route final set direct`. `route final list` names the final, its address and the DNS final; `route chain list` marks the final chain, and `route test` says where traffic goes when no rule matches.

Compiled rules follow shell-proxy: rules that differ only in their users become one rule naming every user, rule-set rules with the same outbound and users merge within a family, and each family (explicit domains first, then geosite, geoip, other) is sorted by outbound, users and rule sets, with each rule's tags sorted. Each chain has one DNS server named `<tag>-dns` in shell-proxy's shape (`tag`, `type`, `server`, `server_port`, `path`, `tls`, `detour`); the families its lookups ask for are set on the DNS rules. A sync renames an older `gproxy-chain-<tag>` server and moves its `domain_strategy` into the chain record.

`route chain add --dns` picks the resolver a chain's lookups go to, reached through the chain itself; without it a chain uses DNS over HTTPS to Google at the address family the chain reaches: `8.8.8.8`, or `2001:4860:4860::8888` for a chain reached only over IPv6. A bare address is plain DNS (`--dns 10.0.0.53`, `--dns 10.0.0.53:5353`); a scheme selects the transport (`udp`, `tcp`, `tls`, `https`, `h3`). The address dialled is always an IP: a chain's DNS server carries a detour and sing-box will not resolve the resolver's own hostname through one, so `--dns https://dns.quad9.net/dns-query` is pinned once to an address of the chain's own family and the name is kept as the certificate name. `route chain list` names the resolver each chain uses.

`route chain modify <tag>` changes where an existing chain points without touching the rules that select it: `--parameter` replaces host, port and credentials together (`host:port` alone removes the credentials), `--dns` changes the resolver, and a flag not given changes nothing. A chain a rule selects cannot be removed, so this is the way to follow an upstream that moved; the alternative was to point every rule elsewhere, remove the chain, add it back and point the rules home again. The tag is fixed — renaming a chain is a remove and an add — and a modification that names nothing to change is a usage error rather than a silent no-op. A changed host re-decides which address families that chain's lookups ask for; a resolver that was never named again is kept.

A chain's lookups ask for the address families its own endpoint has: an endpoint reachable only over IPv4 asks for `ipv4_only` and one reachable only over IPv6 asks for `ipv6_only`, because the other family's answer would be an address that chain cannot reach. A hostname endpoint answering in both families can carry either and prefers IPv4. An IP endpoint answers for itself; a hostname is resolved once when the chain is added, and one that does not resolve keeps the configured direct strategy rather than being guessed at. `route chain list` names the strategy each chain uses, and the strategy from `route direct set` still governs direct rules.

A SOCKS5 chain is given as `--parameter <host>:<port>[:<username>:<password>]`: `host:port` for an unauthenticated server, `host:port:username:password` otherwise. An IPv6 host is bracketed (`[2001:db8::1]:1080:alice:secret`); everything after the third colon is the password, so a password may contain `:`, a username may not. This flag carries the password on the command line by the operator's choice: it is visible in the process listing while the command runs and stays in the shell history. Errors never echo the value. Firewall status includes current/desired/planned changes; applying rules retains DHCPv6/SSH transport requirements.

## Updates and removal

```bash
gproxy core version --json
gproxy core check --timeout 30s --json
gproxy core update snell --json
gproxy core update sing-box --version 1.13.11 --json
gproxy update --check --timeout 30s --json
gproxy uninstall --preview --json
```

Core selectors are `sing-box`, `snell`, `shadow-tls`, `caddy`; `core update --all` updates installed cores sequentially. Snell currently uses the verified `6.0.0rc2` archive, which identifies itself as `v6.0.0`; `core version` and `core check` both report the archive version, read from the receipt written beside the binary. `core check` carries `installed` beside `update_available`: a core that is not installed has nothing to update, so `update_available` is `false` and `installed` is what says a first install is available. Update commands validate integrity/version and replace binaries atomically; checks do not install anything. Unversioned self-update never downgrades a newer/development build to an older stable release. An explicit `--version` deliberately selects that release.

Help follows the same rule: `gproxy`, `gproxy <command> --help` and `gproxy help <command>` list each command as `name (what it does)` and each flag as `--flag type (what it does; default x)`, with no blank lines between the sections. Every bracketed note in human output is lowercase, drawn on a terminal in one lighter grey brackets included (a certificate warning keeps its red text inside them), and follows its value after one space — `v0.3.1 (already latest version)`, `1.14.1 -> v1.14.2 (update available)`, `sing-box (running)`. `gproxy update --check` and `gproxy update` answer in one line: `go-proxy-cli version: v0.3.1 (already latest version)`, `v0.3.0 -> v0.3.1 (updates available)` (or `(downgrade available)` when `--version` names an older release), or after an update `v0.3.0 -> v0.3.1 (updated)`; a development build says `(development build; latest <version>)`. `data.updated` is true once the executable was replaced. Only run `gproxy update` or `gproxy uninstall --confirm` when replacement/removal is actually intended. `gproxy uninstall` with neither flag prints its two forms, `--preview` and `--confirm`, and exits 2. `--preview` lists only what exists on this host, one path per line grouped by folder (each folder in its own colour on a terminal), then the `/etc/bash.bashrc` completion block and the `inet proxy_firewall` table when they are present. Uninstall removes owned configuration, units, binaries, firewall state and the coordination directory `/run/lock/go-proxy`, and clears the systemd failure records of its own units by name so none is left showing as `not-found` in `systemctl --failed`. It does not touch unrelated system journals, services or failure records. `init` installs a tmpfiles rule so that directory exists again after a reboot, which is what lets read-only commands take a lock.

## Guidance and human output

A command that names an action but cannot perform it prints what the next choice is, instead of a bare flag error:

```bash
gproxy protocol add            # command reference: every protocol with its flags
gproxy protocol add vless      # prints the one vless command with every flag it takes
gproxy server restart          # lists the selectable services
gproxy route rule add          # prints the numbered preset menu
```

Guidance is commands only, flush left, with no `error:` line above them: the full command with every option it takes, closed sets of values written inside it (`<ipv4_only|ipv6_only|...>`), then one or two examples that run as written, or a numbered menu whose indexes the command takes. It is meant to be copied off the screen and edited. An error with no commands to show after it, such as `unknown preset x` or a port out of range, keeps its `error:` line.

This is a **usage error, not a result**: it goes to stderr, exits **2**, writes nothing to stdout, and under `--json` returns the ordinary `invalid_argument` envelope, with the message the human output leaves out and the same choices in `data` (`data.protocols`, `data.services`, `data.presets`, `data.missing`). Never parse the human guidance; read `data`.

`protocol list` answers what can be **installed**: one row per protocol with the forms it comes in, and `data.protocols` under `--json`. It reads no state, so it is the one listing that needs neither root nor an initialised runtime.

What **is** installed is read two ways. `protocol remove` with no argument lists every node: its name -- the tag without the port, which is already the next column -- then the port, the layer that encrypts it, and who it belongs to. A node wrapped in ShadowTLS names the wrapper in the encryption column; the only port a listing names is the node's own. It takes the row number it printed, the name it printed beside that number, or the full tag, which is what `--json` reports and what a script holds. A name shared by two nodes of one protocol on different ports is refused with both ports rather than guessed at. That listing is guidance, so it goes to stderr at exit **2**: it is what a person reads, not a result to parse. An agent reads the inventory from the owner instead — `user list --json` carries every membership's tag, protocol and port — or from the `data` of the installation itself.

Human-readable output uses one layout everywhere: rows numbered from one, labels left-aligned with no indent, detail in a second column. Service names are the short ones (`snell`, `caddy`, `watchdog`), and a managed service is reported as `running` or `stopped` only — whether a stopped unit is installed at all is carried by colour, not by another word. Commands without a rendering still emit indented JSON.

Human-readable output is coloured when stdout is a terminal. Colour is suppressed for pipes, files and any non-terminal, and by `NO_COLOR`, `TERM=dumb` or `--no-color`. `--json` never carries escape sequences under any of these conditions, so an agent needs no ANSI stripping. That includes foreign text this CLI only relays: a log is stripped of escape sequences where it is read, because sing-box and caddy colour their own output and the JSON path has no renderer to strip it later.

## Shell completion

The installer writes the bash and zsh scripts (`/usr/share/bash-completion/completions/gproxy`, `/usr/share/zsh/site-functions/_gproxy`). Debian and Ubuntu load bash completion only in login shells (`sudo -i`, `su -`), so the installer also adds one marked block to `/etc/bash.bashrc` that loads bash-completion in every interactive shell (`sudo su`, `sudo -s`); it is added once, skipped when the distribution's own loader is already active, and removed by `uninstall`. For another shell, or a build from source:

```bash
gproxy completion zsh > "${fpath[1]}/_gproxy"      # or bash / fish / powershell
```

Completion covers subcommands and every argument whose values are a closed set: protocol types, configuration sources, managed services, core components, routing preset indexes (each described by its preset), direct strategies, a firewall port's transport once its number is typed (`8443<Tab>` offers `8443/tcp`, `8443/udp`, `8443/both`), SS methods and TUIC congestion. Flags taking a free-form value, `--parameter` included, offer nothing rather than file names. In bash, several candidates are listed one per line as `name (description)`, the description in lowercase; `gproxy completion bash` appends that formatting to cobra's script, and `--no-descriptions` omits both.

Values that come from runtime state — user names, node tags, chain tags, rule indexes — are deliberately **not** completed. Reading them requires the state lock and root, while a completion script runs as the invoking user, so it would contend with a running mutation and still return nothing. Those values are surfaced by the guidance above, inside the command that needs them.

A completion request costs what `--help` costs (measured 5 ms against 4 ms on a 2 vCPU target) because it is answered from the in-memory command tree without loading state. Agents are unaffected: they pass argv directly and never trigger completion.

## Agent execution checklist

1. Read `version --json`, command help, and the relevant current-state query.
2. Build an explicit request with the correct user/tag/port and flags; never simulate keyboard input.
3. Capture stdout, stderr and exit status independently. Parse JSON only when requested and the output is complete.
4. Inspect `ok`, `changed`, error stage and actual postconditions. Validate configuration, required service/listener state and compatible client traffic for networking changes.
5. Retry only after classifying `busy`, conflict or pending activation. Preserve credentials and unrelated resources.
6. Keep exports private, terminate follow/test-client processes and remove only test-owned artifacts.
