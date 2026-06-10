# Building scproxy

## Quick Start

### Build for current platform
```bash
make build
```

### Build for all platforms
```bash
make build-all
# Or run the script directly
./build.sh
```

## Build Platforms

The build script supports the following platforms:

| Platform | Architecture | Output |
|----------|-------------|--------|
| Linux | amd64, arm64 | `scproxy-linux-amd64.tar.gz` |
| Windows | amd64, arm64 | `scproxy-windows-amd64.zip` |
| macOS | amd64, arm64 | `scproxy-darwin-amd64.tar.gz` |

## Build Output

All build artifacts are placed in the `output/` directory:

```
output/
├── scproxy-linux-amd64.tar.gz       # Linux 64-bit
├── scproxy-linux-arm64.tar.gz       # Linux ARM64
├── scproxy-windows-amd64.zip        # Windows 64-bit
├── scproxy-windows-arm64.zip        # Windows ARM64
├── scproxy-darwin-amd64.tar.gz      # macOS Intel
├── scproxy-darwin-arm64.tar.gz      # macOS Apple Silicon
└── SHA256SUMS.txt                # Checksums for all archives
```

## Installation

### Linux
```bash
# Download and extract
tar -xzf scproxy-linux-amd64.tar.gz
sudo cp scproxy /usr/local/bin/
sudo chmod +x /usr/local/bin/scproxy
```

### Windows
```powershell
# Download and extract
unzip scproxy-windows-amd64.zip
# Add to PATH or copy to a directory in PATH
```

### macOS
```bash
# Download and extract
tar -xzf scproxy-darwin-amd64.tar.gz
sudo cp scproxy /usr/local/bin/
sudo chmod +x /usr/local/bin/scproxy
```

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build for current platform |
| `make build-all` | Build for all platforms |
| `make clean` | Clean build artifacts |
| `make test` | Run tests |
| `make test-cover` | Run tests with coverage |
| `make run` | Build and run with --help |
| `make dev` | Build for development |
| `make dev-run` | Build and run with debug logging |
| `make deps` | Download dependencies |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make release` | Create release packages |
| `make help` | Show all available commands |

## Custom Build Variables

You can customize the build with environment variables:

```bash
# Set version
VERSION=v1.0.0 ./build.sh

# Set output directory
OUTPUT_DIR=dist ./build.sh

# Combined
VERSION=v1.0.0 OUTPUT_DIR=dist ./build.sh
```

## Development

### Build and run locally
```bash
# Build for your current platform
make build

# Run with debug logging
./scproxy --target https://example.com --proxy http://proxy:8080 --port 8080 --log-level debug

# Or use the dev-run target
make dev-run
```

### Run tests
```bash
# Run all tests
make test

# Run with coverage
make test-cover
```

## Verification

After building, verify the binary:

```bash
# Check version
./scproxy --version

# Check help
./scproxy --help

# Verify binary
file scproxy
# Linux: ELF 64-bit LSB executable...
# Windows: PE32+ executable (console) x86-64...
# macOS: Mach-O 64-bit executable...
```

## Build Requirements

- Go 1.24.3 or later
- Make (for Makefile commands)
- Standard build tools (tar, zip, sha256sum)

## Cross-Compilation Notes

The build script uses Go's cross-compilation features by setting `GOOS` and `GOARCH` environment variables. This works seamlessly for:

- Linux -> Linux (all architectures)
- Linux -> Windows (all architectures)
- Linux -> macOS (all architectures)

No external cross-compilation tools are required.

## Troubleshooting

### Build fails
```bash
# Ensure dependencies are installed
make deps

# Clean and rebuild
make clean
make build
```

### Permission denied on Linux
```bash
# Make binary executable
chmod +x scproxy
```

### Windows SmartScreen warning
Windows may show a SmartScreen warning for unsigned binaries. Click "More info" → "Run anyway".

## CI/CD Integration

### GitHub Actions
```yaml
- name: Build
  run: |
    chmod +x build.sh
    ./build.sh
```

### GitLab CI
```yaml
build:
  script:
    - chmod +x build.sh
    - ./build.sh
```

## Release Checklist

- [ ] Update version in code
- [ ] Run `make clean`
- [ ] Run `make test`
- [ ] Run `make build-all`
- [ ] Verify checksums
- [ ] Test binaries on target platforms
- [ ] Create Git tag
- [ ] Push to repository
