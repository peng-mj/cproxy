# Scproxy Scripts

This directory contains utility and test scripts for scproxy.

## Automated Testing

### Run All Tests (Recommended)

The easiest way to run all tests is using the automated test runner in the project root:

```bash
./run_all_tests.sh
```

This script will:
- **Auto-detect your system** (OS, CPU architecture, Go version)
- **Automatically build** a temporary scproxy binary optimized for your system
- **Run all test scripts** in sequence
- **Clean up automatically** after testing (removes temporary binary and cache)
- **Display results** with color-coded output and statistics

**No manual build required!** The test runner handles everything.

#### System Detection

The test runner automatically detects:
- **Operating System**: Linux, macOS, Windows, etc. (via `go env GOOS`)
- **CPU Architecture**: amd64, arm64, arm, etc. (via `go env GOARCH`)
- **Architecture variants**: MIPS variants, ARM versions (if applicable)
- **Go Version**: Current Go installation
- **Host System**: Linux distribution or macOS version

Example output:
```
========================================
Building Test Binary
========================================

[INFO] Detecting system and architecture...
[SUCCESS] Detected OS: linux
[SUCCESS] Detected Architecture: amd64
[INFO] Go version: go1.24.3
[INFO] Host system: Ubuntu 24.04

[INFO] Building temporary scproxy binary for testing...
[INFO] Output: .scproxy_test
[INFO] Target: linux/amd64

  github.com/Madh93/scproxy
  github.com/Madh93/scproxy/internal/cache
  ...

[SUCCESS] Temporary binary built successfully
[INFO] Binary size: 15473024 bytes (~14.75 MB)
[INFO] Binary type: ELF 64-bit LSB executable, x86-64
[SUCCESS] Binary verified and executable
```

### What Gets Cleaned Up

The automated test runner creates and removes:
- **Temporary binary**: `.scproxy_test` (removed after testing)
- **Test cache directories**: `.test_cache/` (removed after testing)
- **Temporary test directories**: `/tmp/scproxy_test` (removed after testing)
- **Test log files**: `scproxy_test.log`, `test_output.txt`, etc. (removed after testing)

This ensures each test run starts with a clean environment and doesn't leave artifacts behind.

## Available Scripts

### Cache Management

#### `cache.py`
Cache management tool for scproxy. Provides commands to enable, disable, and view cache status.

**Usage:**
```bash
python scripts/cache.py <command>
```

**Commands:**
- `enable` - Enable cache mode
- `disable` - Disable cache mode
- `status` - View cache status and statistics
- `clear` - Clear cached data
- `help` - Show help message

**Examples:**
```bash
python scripts/cache.py enable   # Enable cache
python scripts/cache.py status   # View cache status
python scripts/cache.py clear    # Clear cache
```

### Testing Scripts

#### `test_cache.py`
Integration test script for scproxy's caching functionality using Ubuntu package files from Aliyun mirror.

**Usage:**
```bash
python scripts/test_cache.py
```

**Test Coverage:**
- Server startup and shutdown
- Cache miss scenario (first download)
- Cache hit scenario (second download)
- Multiple file cache management
- Cache file integrity verification
- Configuration persistence

**Test Files:**
- `libaal_1.0.5-6.dsc` (1.3 KB)
- `libaal_1.0.6-1.dsc` (~1 KB)

#### `test_diverse_cache.py`
Enhanced integration test with diverse file types, sources, and sizes. Tests cache behavior across different scenarios.

**Usage:**
```bash
python scripts/test_diverse_cache.py
```

**Test Coverage:**
- **Tiny files** (< 2KB): Ubuntu package descriptions
- **Small files** (< 10KB): GitHub checksums
- **Medium files** (< 100KB): Debian constitution text
- **Large files** (> 100KB): GitHub release binaries
- **Multiple sources**: Aliyun mirrors, GitHub releases
- **Multiple file types**: `.dsc`, `.txt`, `.tar.gz`
- **Cache MISS/HIT verification** for each category
- **Performance comparison** between cached and uncached downloads

**Test Files:**
1. `libaal_1.0.5-6.dsc` (1.3 KB) - Tiny package description
2. `constitution.txt` (31 KB) - Medium text file
3. `checksums.txt` (~800 B) - Small GitHub file
4. `dufs-v0.45.0-x86_64-unknown-linux-musl.tar.gz` (~1.5 MB) - Large binary archive

#### `test_batch_demo.py`
Demo script showcasing scproxy's batch proxy configuration functionality. Tests multiple routes on different ports simultaneously.

**Usage:**
```bash
python scripts/test_batch_demo.py
```

**Requirements:**
- Requires `test-config.json` in project root
- Tests ports 18081, 18082, and 18083
- Demonstrates configuration file + CLI parameter combination

#### `test_github_releases.py`
Test script for GitHub releases proxy optimization feature. Verifies that scproxy correctly handles GitHub releases URLs with optimized chunking and caching.

**Usage:**
```bash
python scripts/test_github_releases.py
```

**Test Coverage:**
- First download (cache miss)
- Second download (cache hit)
- File integrity verification
- GitHub releases handler usage detection
- Cache directory verification

## Requirements

All scripts require:
- Python 3.6 or higher
- Compiled `scproxy` binary in project root
- Standard library only (no external Python dependencies)

Build scproxy first:
```bash
make build
```

## Running Tests

Run all tests:
```bash
# Test cache functionality
python scripts/test_cache.py

# Test batch proxy mode
python scripts/test_batch_demo.py

# Test GitHub releases optimization
python scripts/test_github_releases.py
```

## Notes

- All scripts automatically clean up after themselves (stop servers, remove temporary files)
- Scripts use signal handlers for graceful shutdown
- Test scripts provide colored output for better readability
- Logs are saved to `/tmp/` or current directory for debugging