<p align="center">
  <img src="./docs/img/logo.png" alt="prxy logo" width="75%" height="75%>
</p>
<p align="center">
  </br>
  <b>prxy</b> is a versatile HTTP reverse proxy written in <a href="https://go.dev/">Go</a> that supports batch proxy configuration, optional outbound proxy routing, automatic Host header rewriting, and intelligent HTTP response caching.
</p>
<hr>

[![Latest release](https://img.shields.io/github/v/tag/Madh93/prxy?label=Release)](https://github.com/Madh93/prxy/releases)
[![Go Version](https://img.shields.io/badge/Go-1.24-blue)](https://go.dev/doc/install)
[![Go Report Card](https://goreportcard.com/badge/github.com/Madh93/prxy)](https://goreportcard.com/report/github.com/Madh93/prxy)
[![Build Status](https://img.shields.io/github/actions/workflow/status/Madh93/prxy/continuous-integration.yml?branch=main)](https://github.com/Madh93/prxy/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/Madh93/prxy.svg)](https://pkg.go.dev/github.com/Madh93/prxy)
[![License](https://img.shields.io/badge/License-MIT-brightgreen)](LICENSE)

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#caching">Caching</a> •
  <a href="#development">Development</a> •
  <a href="#license">License</a>
  <br>
  <a href="README_CN.md">简体中文</a>
</p>

## Features

### Core Functionality

- **Batch Proxy Configuration**: Run multiple proxy services simultaneously, each listening on a different port and forwarding to a different target URL
- **Outbound Proxy Support**: Route all proxy traffic through an external HTTP proxy (e.g., wireproxy, Squid)
- **Automatic Host Header Rewriting**: Ensures requests reach the correct destination service
- **Graceful Shutdown**: Handles SIGTERM/SIGINT signals for clean shutdown

### Advanced Caching

- **Path-Based Storage**: Cache files are stored using URL paths for easy inspection
- **Streaming Cache Support**: Efficiently handle large files with streaming writes
- **Range Request Support**: HTTP Range requests are fully supported for partial content delivery
- **Configurable Cache Policies**: 
  - File size limits (min/max)
  - Total cache size limit
  - Extension-based exclusions
  - Authenticated request caching (optional)
- **GitHub Releases Optimization**: Special handling for GitHub release downloads with improved performance
- **Cache Statistics**: Track cache hits, misses, and storage usage

### Configuration Management

- **Multiple Configuration Sources**: CLI flags, environment variables, and config files
- **Configuration Persistence**: Automatically saves settings to config file
- **Flexible Route Management**: Add routes via CLI or config file
- **Dynamic Route Addition**: Temporarily add routes without editing config file

## Installation

### From Binary

Download the latest binary from [releases](https://github.com/Madh93/prxy/releases):

```bash
curl -L https://github.com/Madh93/prxy/releases/latest/download/prxy_$(uname -s)_$(uname -m).tar.gz | tar -xz -O prxy > /usr/local/bin/prxy
chmod +x /usr/local/bin/prxy
```

### From Source

```bash
go install github.com/Madh93/prxy@latest
```

### Docker

```bash
docker run --name prxy ghcr.io/madh93/prxy:latest --proxy http://proxy:8080
```

### Docker Compose

```yaml
services:
  prxy:
    image: ghcr.io/madh93/prxy:latest
    restart: unless-stopped
    volumes:
      - ./config.json:/root/cache/prxy.json:ro
      - ./cache:/root/cache
    environment:
      - PRXY_PROXY=http://proxy:8080
```

## Quick Start

### Single Target Mode

```bash
prxy --target https://example.com --proxy http://localhost:8080 --port 8080
```

### Batch Mode (Recommended)

Create a configuration file at `./cache/prxy.json`:

```json
{
  "host": "0.0.0.0",
  "proxy": "http://localhost:8080",
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

Start the service:

```bash
prxy
```

### Add Temporary Route

Add a route via CLI without editing the config file:

```bash
prxy --target https://httpbin.org --port 9999
```

## Configuration

### Configuration File

The configuration file is automatically created at `./cache/prxy.json` on first run. The default structure:

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
    "excludeExtensions": ["html", "js", "css", "json", "xml"]
  },
  "logging": {
    "level": "info",
    "format": "text",
    "output": "stdout"
  }
}
```

### Configuration Precedence

Configuration values are loaded from multiple sources (highest to lowest priority):

1. **Command-line flags**
2. **Environment variables** (`PRXY_*` prefix)
3. **Configuration file** (`./cache/prxy.json`)
4. **Default values**

### CLI Flags

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--config`, `-c` | `PRXY_CONFIG` | Config file path | `./cache/prxy.json` |
| `--target`, `-t` | `PRXY_TARGET` | Target service URL | N/A |
| `--port`, `-P` | `PRXY_PORT` | Port to listen on | N/A |
| `--proxy`, `-x` | `PRXY_PROXY` | Outbound HTTP proxy URL | Direct connection |
| `--host`, `-H` | `PRXY_HOST` | Host to listen on | `0.0.0.0` |
| `--cache` | `PRXY_CACHE` | Enable caching | `true` |
| `--clear-cache` | N/A | Clear cache and exit | `false` |
| `--yes` | `PRXY_YES` | Auto-confirm cache clearing | `false` |
| `--log-level`, `-l` | `PRXY_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `--log-format`, `-f` | `PRXY_LOG_FORMAT` | Log format (text/json) | `text` |
| `--log-output`, `-o` | `PRXY_LOG_OUTPUT` | Log output (stdout/stderr/file) | `stdout` |

**Note:** `--target` and `--port` add additional routes instead of replacing existing ones. Duplicate ports are automatically skipped with a warning.

## Caching

### Cache Configuration

Cache behavior is controlled via the `cache` section in the configuration file:

- **enabled**: Enable or disable caching
- **directory**: Storage directory for cached files
- **maxTotalSizeMB**: Maximum total cache size (0 = no limit, LRU eviction enabled when > 0)
- **minFileSizeKB**: Minimum file size to cache (0 = no limit)
- **maxFileSizeKB**: Maximum file size to cache (0 = no limit)
- **cacheAuth**: Whether to cache authenticated requests
- **excludeExtensions**: File extensions to exclude from caching

### Cache Management

View cache statistics:

```bash
# Cache statistics are automatically logged
# Check response headers for cache status:
# X-Cache: HIT     - Response from cache
# X-Cache: MISS    - Response forwarded to target
# X-Cache: BYPASS  - Request not cached (excluded by policy)
```

Clear cache:

```bash
# Interactive confirmation
prxy --clear-cache

# Skip confirmation
prxy --clear-cache --yes
```

### Cache Storage

Cached files are stored using URL paths for easy inspection:

```
cache/
├── prxy.json          # Configuration file
└── data/
    ├── github.com/
    │   └── releases/
    │       └── file.tar.gz
    └── api.example.com/
        └── endpoint.json
```

### GitHub Releases Optimization

GitHub releases downloads are automatically optimized with:
- Efficient streaming cache writes
- Automatic redirect following
- Improved connection handling

## Development

### Build

```bash
make build              # Build for current platform
make build-all          # Build for all platforms
make dev                # Quick development build
```

### Test

```bash
make test               # Run all tests
make test-cover         # Run tests with coverage
./run_all_tests.sh      # Run integration tests
```

### Code Quality

```bash
make lint               # Run golangci-lint
make fmt                # Format code
make vet                # Run go vet
```

### Development Workflow

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and linting
5. Submit a pull request

## Architecture

### Package Structure

```
internal/
├── cache/      # HTTP caching layer with path-based storage
├── config/     # Configuration management with file persistence
├── logging/    # Structured logging wrapper around slog
├── prxy/       # Core reverse proxy logic and batch management
└── validation/ # URL and configuration validation
```

### Key Components

- **PrxyManager**: Manages multiple proxy server instances
- **Prxy**: Individual proxy server handling HTTP requests
- **Cache**: HTTP response cache with streaming support
- **Config**: Configuration loading and validation

### Dependencies

- Go 1.24.3 or later
- Only one external dependency: `github.com/spf13/pflag`

## License

This project is licensed under the [MIT license](LICENSE).