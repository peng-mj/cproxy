<p align="center">
  <img src="./docs/img/logo.png" alt="scproxy logo" width="60%" height="60%">
</p>
<p align="center">
  </br>
  <b>scproxy</b> — A command-line HTTP reverse proxy with intelligent caching
</p>
<p align="center">
  Forked from <a href="https://github.com/Madh93/prxy">Madh93/prxy</a> with enhanced features
</p>

<p align="center">
  <i>scproxy stands for <b>Sieve Cache Proxy</b> — filtering and caching HTTP responses efficiently.</i>
</p>

[![Latest release](https://img.shields.io/github/v/tag/peng-mj/scproxy?label=Release)](https://github.com/peng-mj/scproxy/releases)
[![Go Version](https://img.dev/badge/Go-1.24-blue)](https://go.dev/doc/install)
[![Go Report Card](https://goreportcard.com/badge/github.com/peng-mj/scproxy)](https://goreportcard.com/report/github.com/peng-mj/scproxy)
[![Build Status](https://img.shields.io/github/actions/workflow/status/peng-mj/scproxy/continuous-integration.yml?branch=main)](https://github.com/peng-mj/scproxy/actions)
[![License](https://img.shields.io/badge/License-MIT-brightgreen)](LICENSE)

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#caching">Caching</a> •
  <a href="#license">License</a>
  <br>
  <a href="README_CN.md">简体中文</a>
</p>

---

## Overview

**scproxy**(Sieve Cache Proxy) is a lightweight HTTP reverse proxy written in Go that helps you forward HTTP requests through an outbound proxy with intelligent response caching. It's designed for scenarios where you need to:
- Route multiple services through different local ports
- Forward traffic via an external HTTP proxy (e.g., corporate proxies, wireproxy, Squid)
- Cache HTTP responses to reduce bandwidth and improve latency

---

## Features

### Core

- **Batch proxy mode** — Run multiple routes simultaneously on different ports
- **Outbound proxy chaining** — Forward all traffic through an external HTTP proxy
- **Automatic Host header rewriting** — Ensures requests reach the correct backend
- **Graceful shutdown** — Clean handling of SIGTERM/SIGINT signals

### Caching

- **Path-based storage** — Cache files mirror URL paths for easy inspection
- **Streaming support** — Efficient handling of large files
- **HTTP Range requests** — Full support for partial content delivery
- **Configurable policies** — Size limits, extension exclusions, auth handling
- **LRU eviction** — Automatic cache cleanup when size limit is reached
- **Cache statistics** — Track hits, misses, and storage usage

### Configuration

- **Multiple sources** — CLI flags, config files, and defaults
- **Dynamic routes** — Add routes via CLI without editing config
- **Auto-save** — CLI settings persist to config file

---

## Installation

### Binary (Recommended)

```bash
curl -L https://github.com/peng-mj/scproxy/releases/latest/download/scproxy_$(uname -s)_$(uname -m).tar.gz | tar -xz -O scproxy > /usr/local/bin/scproxy
chmod +x /usr/local/bin/scproxy
```

### From Source

```bash
go install github.com/peng-mj/scproxy@latest
```

---

## Quick Start

### Single Route

Forward one target through an outbound proxy:

```bash
scproxy --target https://example.com --proxy http://proxy:8080 --port 8080
```

### Batch Mode (Multiple Routes)

Create a config at `./cache/scproxy.json`:

```json
{
  "host": "0.0.0.0",
  "proxy": "http://proxy:8080",
  "routes": [
    {"target": "https://github.com", "port": 8081},
    {"target": "https://gitlab.com", "port": 8082},
    {"target": "https://api.example.com", "port": 8083}
  ],
  "cache": {
    "enabled": true,
    "directory": "./cache"
  }
}
```

Then run:

```bash
scproxy
```

### Add Temporary Route

```bash
scproxy --target https://httpbin.org --port 9999
```

---

## Configuration

### Config File

Created automatically at `./cache/scproxy.json` on first run:

```json
{
  "host": "0.0.0.0",
  "proxy": "",
  "routes": [],
  "cache": {
    "enabled": true,
    "directory": "./cache",
    "maxTotalSizeMB": 0,
    "minFileSizeKB": 0,
    "maxFileSizeKB": 0,
    "cacheAuth": false,
    "excludeExtensions": ["html", "js", "css", "json", "xml"],
    "excludePaths": []
  },
  "logging": {
    "level": "info",
    "format": "text",
    "output": "stdout"
  }
}
```

### Priority

1. CLI flags (highest)
2. Config file
3. Defaults (lowest)

### CLI Options

| Flag | Description | Default |
|------|-------------|---------|
| `--target`, `-t` | Target service URL | — |
| `--port`, `-P` | Listen port | — |
| `--proxy`, `-x` | Outbound proxy URL | Direct |
| `--host`, `-H` | Listen host | 0.0.0.0 |
| `--config`, `-c` | Config file path | ./cache/scproxy.json |
| `--cache` | Enable caching | true |
| `--clear-cache` | Clear cache and exit | — |
| `--yes` | Skip confirmation | — |
| `--summary`, `-s` | Show cache stats | — |
| `--version`, `-v` | Show version | — |
| `--log-level`, `-l` | Log level | info |
| `--log-output`, `-o` | Log output | stdout |

---

## Caching

### Cache Control

| Setting | Description |
|---------|-------------|
| `enabled` | Enable/disable caching |
| `directory` | Storage path |
| `maxTotalSizeMB` | Total size limit (0 = unlimited, enables LRU when set) |
| `minFileSizeKB` | Min file size to cache |
| `maxFileSizeKB` | Max file size to cache |
| `cacheAuth` | Cache authenticated requests |
| `excludeExtensions` | File extensions to skip |
| `excludePaths` | URL path patterns to skip |

### Path Exclusion

`excludePaths` supports two modes:

| Pattern | Matches |
|---------|---------|
| `/ubuntu/dists/` | All paths under this directory (prefix) |
| `/etc/resolv.conf` | Only this file (exact) |

Rules:
- Must start with `/`
- Trailing `/` = prefix match (directory)
- No trailing `/` = exact match (file)

Example:

```json
{
  "cache": {
    "excludePaths": [
      "/ubuntu/dists/",
      "/pypi/simple/",
      "/etc/resolv.conf"
    ]
  }
}
```

### Management

View stats:

```bash
scproxy --summary
```

Clear cache:

```bash
scproxy --clear-cache          # Interactive
scproxy --clear-cache --yes    # Skip confirmation
```

Response headers indicate cache status:

- `X-Cache: HIT` — Served from cache
- `X-Cache: MISS` — Fetched from target
- `X-Cache: BYPASS` — Not cached (excluded by policy)

### Storage Structure

```
cache/
├── scproxy.json              # Config file
└── data/
    ├── github.com/
    │   └── releases/
    │       └── file.tar.gz
    └── api.example.com/
        └── endpoint.json
```

---

## Development

```bash
make build          # Build for current platform
make build-all      # Build for all platforms
make test           # Run tests
make lint           # Run linters
```

---

## License

[MIT](LICENSE)
