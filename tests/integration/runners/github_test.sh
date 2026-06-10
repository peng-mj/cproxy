#!/bin/bash
# scproxy integration test - GitHub releases & API
# Container: curlimages/curl
# scproxy route: 18801 -> https://github.com
set -e

RESULT_FILE="/results/github_results.json"
LOG_FILE="/results/github.log"

echo "[]" > "$RESULT_FILE"

log() { echo "[$(date '+%H:%M:%S')] $1" | tee -a "$LOG_FILE"; }

run_test() {
    local test_name=$1
    local command=$2

    log "--- Running: $test_name ---"
    START=$(date +%s%N)
    set +e
    OUTPUT=$(eval "$command" 2>&1)
    EXIT_CODE=$?
    set -e
    END=$(date +%s%N)
    DURATION=$(( (END - START) / 1000000000 ))

    echo "$OUTPUT" >> "$LOG_FILE"

    STATUS="PASS"
    [ $EXIT_CODE -ne 0 ] && STATUS="FAIL"

    log "$STATUS: $test_name (${DURATION}s, exit=$EXIT_CODE)"

    # Truncate output for JSON (max 500 chars)
    OUTPUT_SHORT=$(echo "$OUTPUT" | head -20 | tr '\n' ' ' | cut -c1-500)

    jq --arg name "$test_name" \
       --arg status "$STATUS" \
       --arg duration "$DURATION" \
       --arg exit_code "$EXIT_CODE" \
       --arg output "$OUTPUT_SHORT" \
       '. += [{"test": $name, "status": $status, "duration": ($duration|tonumber), "exit_code": ($exit_code|tonumber), "output": $output}]' \
       "$RESULT_FILE" > "${RESULT_FILE}.tmp" && mv "${RESULT_FILE}.tmp" "$RESULT_FILE"
}

PROXY_HOST="${PROXY_HOST:-host.docker.internal}"
GITHUB_PORT="${GITHUB_PORT:-18801}"

SCPROXY="http://${PROXY_HOST}:${GITHUB_PORT}"

log "GitHub proxy: $SCPROXY"

# Install jq (Debian/Alpine compatible)
if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq jq >/dev/null 2>&1 || true
elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache jq > /dev/null 2>&1 || true
fi

# --- Tests ---

# Test 1: Download a small release asset (scproxy project itself)
run_test "github_release_download" "curl -sfL -o /tmp/scproxy.tar.gz ${SCPROXY}/Madh93/prxy/archive/refs/tags/v0.1.0.tar.gz && ls -lh /tmp/scproxy.tar.gz"

# Test 2: Download same file again (cache HIT)
rm -f /tmp/scproxy.tar.gz
run_test "github_release_download_cached" "curl -sfL -o /tmp/scproxy.tar.gz ${SCPROXY}/Madh93/prxy/archive/refs/tags/v0.1.0.tar.gz && ls -lh /tmp/scproxy.tar.gz"

# Test 3: Check cache response headers on second download
run_test "github_cache_headers" "curl -sI ${SCPROXY}/Madh93/prxy/archive/refs/tags/v0.1.0.tar.gz | head -5"

# Test 4: Download README via raw.githubusercontent.com (redirect test)
run_test "github_raw_file" "curl -sfL -o /tmp/readme.md ${SCPROXY}/Madh93/prxy/raw/main/README.md && wc -l /tmp/readme.md"

# Test 5: API request - repo info (note: api.github.com is a different host,
# so we test downloading a raw file from the repo instead which goes through github.com)
run_test "github_api_repo" "curl -sfL ${SCPROXY}/Madh93/prxy/blob/main/LICENSE?raw=true -o /tmp/license.txt && wc -c /tmp/license.txt"

# Test 6: Range request (first 1KB)
run_test "github_range_request" "curl -sfL -r 0-1023 -o /tmp/part.bin ${SCPROXY}/Madh93/prxy/archive/refs/tags/v0.1.0.tar.gz && ls -lh /tmp/part.bin"

# Test 7: Latest release redirect
run_test "github_latest_redirect" "curl -sI ${SCPROXY}/Madh93/prxy/releases/latest | head -5"

log "=== GitHub tests completed ==="
