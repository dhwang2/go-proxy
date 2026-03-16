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
proxy menu|status|start|stop|restart|log|update|version
proxy config view|validate
proxy user list|add|rename|delete
proxy protocol list|install|remove
proxy network status|bbr|firewall
proxy core versions|check|update
proxy routing list|set|clear|sync-dns|test
proxy sub show|target
```

## Directory Structure

```text
go-proxy/
├── app/                        ← Application source code
│   ├── cmd/proxy/main.go       ← CLI entry point
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
