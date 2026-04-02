# go-proxy

Go rewrite — a multi-protocol proxy management CLI tool.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/dhwang2/go-proxy/main/app/install.sh | sudo bash
```

## Local Build

```bash
cd app && make build
cd app && make test
```

## CLI Commands

```
gproxy menu|status|start|stop|restart|log|update|version
gproxy config view|validate
gproxy user list|add|rename|delete
gproxy protocol list|install|remove
gproxy network status|bbr|firewall
gproxy core versions|check|update
gproxy routing list|set|clear|sync-dns|test
gproxy sub show|target
```

## Directory Structure

```text
go-proxy/
├── app/                        ← Application source code
│   ├── cmd/gproxy/main.go      ← CLI entry point
│   ├── internal/               ← Private packages (by domain)
│   │   ├── config/
│   │   ├── core/
│   │   ├── crypto/
│   │   ├── protocol/
│   │   ├── routing/
│   │   ├── service/
│   │   ├── tui/
│   │   └── user/
│   ├── pkg/                    ← Reusable packages
│   ├── go.mod
│   ├── Makefile
│   └── install.sh
└── README.md
```

## Private Dev Repo

The private `go-proxy-dev` repository carries additional AI companion files that are not part of the public mirror:

- `.claude/` — Claude Code rules, skills, and agent definitions
- `.codex/` — Codex-oriented mirrors of repo rules, verification playbooks, and agent-role mappings
