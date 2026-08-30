# Agent Instructions for scproxy

## Commands

### Build
- `make build` - Build for current platform (outputs `./scproxy`)
- `make build-all` - Cross-platform builds via `./build.sh` (outputs to `output/`)

### Test
- `make test` - Run Go unit tests
- `./run_all_tests.sh` - Run all Python integration tests (requires Python 3)
  - Builds temporary binary to `./.scproxy_test` first
  - Runs tests from `scripts/*.py`

### Code Quality
- `make lint` - Run golangci-lint (configured in `.golangci.yml`)
- `make fmt` - Format with `gofmt`
- `make vet` - Run `go vet`

## Architecture

```
internal/
├── cache/       # HTTP caching with path-based storage
├── config/      # Configuration loading and CLI flag handling
├── logging/     # Structured logging (slog wrapper)
├── scproxy/      # Core proxy logic and batch management
├── stats/       # Cache statistics collection
├── validation/  # URL and config validation
└── version/     # Version info
```

## Important Notes

- **Don't use Taskfile.yml** - Has incorrect import paths (`github.com/Madh93/scproxy/internal/version`). Use `make` commands instead.
- **Integration tests require Python 3** - All test scripts in `scripts/` are Python.
- **Default cache directory**: `/data/cache` (falls back to `./cache` without write permission); config file at `./cache/config.json` or `/etc/scproxy/config.json`
- **Version info**: Embedded via ldflags in `make build` (not in `internal/version` package)
- **Go version**: 1.24.3 (from `go.mod`)
- **Only external deps**: `github.com/spf13/pflag`, `github.com/brettski/go-termtables`