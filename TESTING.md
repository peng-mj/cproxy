# Scproxy Cache Functionality Testing

## Test Overview

We have created a comprehensive integration test script `scripts/test_cache.py` to verify scproxy's cache functionality. This script uses the real Aliyun Ubuntu mirror source for testing.

## Test Content

### 1. Test Environment
- **Target server**: https://mirrors.aliyun.com
- **Proxy port**: 9091
- **Test directory**: /tmp/scproxy_test
- **Cache directory**: /tmp/scproxy_test/cache

### 2. Test Files
- conntrack-tools_1.4.9.orig.tar.xz (452 KB)
- dbus-glib_0.100.2-1.debian.tar.gz (48 KB)

### 3. Test Scenarios

#### Scenario 1: First Download (CACHE MISS)
```bash
curl http://127.0.0.1:9091/ubuntu/pool/main/c/conntrack-tools/conntrack-tools_1.4.9.orig.tar.xz
```
**Expected result**: File downloaded from target server and saved to cache
**Verification**: Log shows "Cache miss"

#### Scenario 2: Second Download (CACHE HIT)
```bash
curl http://127.0.0.1:9091/ubuntu/pool/main/c/conntrack-tools/conntrack-tools_1.4.9.orig.tar.xz
```
**Expected result**: File returned from cache without re-downloading
**Verification**: Log shows "Cache hit", faster response

#### Scenario 3: Multiple File Caching
Download different files to verify cache system can correctly handle multiple files

## Running Tests

### Prerequisites
1. Compiled scproxy binary: `go build -o scproxy .`
2. Ensure port 9091 is not in use

### Run Command
```bash
python scripts/test_cache.py
```

### Sample Test Output
```
==================================
Scproxy Cache Functionality Test
==================================

[INFO] Starting scproxy server...
[INFO] Target: https://mirrors.aliyun.com
[INFO] Port: 9091
[SUCCESS] Scproxy server started (PID: 51274)

[INFO] Test 1: First download (should be CACHE MISS)
[SUCCESS] Download completed (Size: 452480 bytes)
[SUCCESS] Cache MISS detected in logs (as expected)

[INFO] Test 2: Verify file was cached
[SUCCESS] Cache files created: 2 file(s)

[INFO] Test 3: Second download (should be CACHE HIT)
[SUCCESS] Download completed (Size: 452480 bytes)
[SUCCESS] Cache HIT detected in logs (as expected)

==================================
Test Summary
==================================
[SUCCESS] All tests completed successfully!

Test results:
  Scproxy server started successfully
  First download (cache MISS)
  File was cached
  Second download (cache HIT)
  Multiple files cached

Cache statistics:
  Files cached: 4
  Total size: 500K
```

## Test Result Analysis

### Cache Hit Verification
The test script checks the log file to verify:
- First download log shows "Cache miss"
- Second download log shows "Cache hit"
- Files correctly saved to cache directory

### Cache File Structure
Cache files are stored using path-based keys with the following directory structure:
```
/tmp/scproxy_test/cache/
├── data/
│   └── ubuntu/
│       └── pool/
│           └── main/
│               └── c/
│                   └── conntrack-tools/
│                       └── conntrack-tools_1.4.9.orig.tar.xz
└── meta/
    └── ubuntu/
        └── pool/
            └── main/
                └── c/
                    └── conntrack-tools/
                        └── conntrack-tools_1.4.9.orig.tar.xz.meta
```

### Performance Comparison
- **First download**: Full file downloaded from target server
- **Cache hit**: Returned directly from local cache, significantly reduced response time

## Manual Testing

You can also test manually:

### 1. Start scproxy server
```bash
./scproxy --target "https://mirrors.aliyun.com" --port 9091 --cache
```

### 2. Test cache miss
```bash
# First request
curl -v http://127.0.0.1:9091/ubuntu/pool/main/c/conntrack-tools/conntrack-tools_1.4.9.orig.tar.xz -o /tmp/test1.tar.xz
```

### 3. Test cache hit
```bash
# Second request (should return from cache)
curl -v http://127.0.0.1:9091/ubuntu/pool/main/c/conntrack-tools/conntrack-tools_1.4.9.orig.tar.xz -o /tmp/test2.tar.xz
```

### 4. Verify file integrity
```bash
# Compare two downloaded files
md5sum /tmp/test1.tar.xz /tmp/test2.tar.xz
# Should show same MD5 value
```

### 5. View cache statistics
```bash
./scproxy --clear-cache
```

## Test Coverage

### Functional Tests
- Cache miss scenario
- Cache hit scenario
- Multiple file cache management
- Cache file integrity
- Correct log recording

### Integration Tests
- Server startup and shutdown
- Config file auto-creation and update
- CLI parameter (--cache) works correctly
- Cache directory structure correct
- Graceful shutdown handling

## Troubleshooting

### Port Already in Use
If you get "address already in use" error:
```bash
# Find process using the port
lsof -i :9091

# Kill the process
kill <PID>
```

### Cache Directory Permission Issues
Ensure you have permission to create cache directory:
```bash
mkdir -p /tmp/scproxy_test/cache
chmod 755 /tmp/scproxy_test/cache
```

### Network Connection Issues
If unable to connect to Aliyun mirror:
```bash
# Test network connection
curl -I https://mirrors.aliyun.com

# Check DNS resolution
nslookup mirrors.aliyun.com
```

## Automated Test Suite

### Run All Tests

For comprehensive testing, use the automated test runner that executes all test scripts:

```bash
./run_all_tests.sh
```

This script will:
1. Check prerequisites (scproxy binary, Python 3)
2. Run all test scripts in sequence
3. Collect and display test results
4. Show summary with pass/fail statistics

### Available Tests

The automated test suite includes:

1. **Cache Functionality Test** (`scripts/test_cache.py`)
   - Tests cache miss/hit scenarios
   - Verifies file integrity
   - Validates multiple file caching

2. **Batch Proxy Demo** (`scripts/test_batch_demo.py`)
   - Demonstrates batch proxy configuration
   - Tests multiple routes on different ports
   - Requires `test-config.json` in project root

3. **GitHub Releases Test** (`scripts/test_github_releases.py`)
   - Tests GitHub releases optimization
   - Verifies chunked downloads
   - Validates caching behavior for GitHub assets

### Test Output

The automated test runner provides:
- Color-coded output (green for success, red for failure)
- Individual test results
- Overall summary with statistics
- Execution time tracking

Example output:
```
========================================
Test 1: Cache Functionality
========================================
Running: Cache Functionality

✓ PASSED: Cache Functionality

========================================
Test Summary
========================================

Total Tests Run: 3
Passed: 3
Failed: 0
Duration: 45s
```

## Continuous Integration

### Individual Test Script

This test script can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions workflow
- name: Run cache tests
  run: |
    go build -o scproxy .
    python scripts/test_cache.py
```

### Automated Test Suite

Or use the automated test runner for comprehensive testing:

```yaml
# Example GitHub Actions workflow
- name: Run all tests
  run: |
    go build -o scproxy .
    ./run_all_tests.sh
```

## Summary

The test script successfully verified scproxy's cache functionality:
- Cache system works correctly
- Files correctly saved and retrieved
- Cache hit/miss logic correct
- Multiple file management works
- Config file auto-update works

All tests passed, scproxy's cache functionality is ready for production use!
