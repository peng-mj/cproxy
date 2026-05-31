#!/bin/bash
# prxy integration test - Go modules
# Container: golang:1.24-bookworm
# prxy route: 18800 -> https://mirrors.aliyun.com
# GOPROXY pointed at prxy (acting as goproxy mirror)
set -e

RESULT_FILE="/results/go_results.json"
LOG_FILE="/results/go.log"

echo "[]" > "$RESULT_FILE"

log() { echo "[$(date '+%H:%M:%S')] $1" | tee -a "$LOG_FILE"; }

run_test() {
    local test_name=$1
    local command=$2

    log "--- Running: $test_name ---"
    START=$(date +%s%N)
    set +e
    eval "$command" >> "$LOG_FILE" 2>&1
    EXIT_CODE=$?
    set -e
    END=$(date +%s%N)
    DURATION=$(( (END - START) / 1000000000 ))

    STATUS="PASS"
    [ $EXIT_CODE -ne 0 ] && STATUS="FAIL"

    log "$STATUS: $test_name (${DURATION}s, exit=$EXIT_CODE)"

    jq --arg name "$test_name" \
       --arg status "$STATUS" \
       --arg duration "$DURATION" \
       --arg exit_code "$EXIT_CODE" \
       '. += [{"test": $name, "status": $status, "duration": ($duration|tonumber), "exit_code": ($exit_code|tonumber)}]' \
       "$RESULT_FILE" > "${RESULT_FILE}.tmp" && mv "${RESULT_FILE}.tmp" "$RESULT_FILE"
}

PROXY_HOST="${PROXY_HOST:-host.docker.internal}"
GO_PORT="${GO_PORT:-18803}"
GOPROXY="http://${PROXY_HOST}:${GO_PORT},direct"
GOSUMDB="sum.golang.google.cn"

log "GOPROXY=$GOPROXY"
log "GOSUMDB=$GOSUMDB"

# Clean previous test data
rm -rf /tmp/gotest
mkdir -p /tmp/gotest
cd /tmp/gotest

# --- Tests ---

# Test 1: init module
run_test "go_mod_init" "go mod init example.com/gotest"

# Test 2: get a small package (first time)
run_test "go_get_spew_first" "go get github.com/stretchr/testify@v1.9.0"

# Test 3: get another package
run_test "go_get_cobra" "go get github.com/spf13/cobra@v1.8.1"

# Test 4: download modules
run_test "go_mod_download" "go mod download"

# Test 5: tidy
run_test "go_mod_tidy" "go mod tidy"

# Test 6: verify
run_test "go_mod_verify" "go mod verify"

# Test 7: list modules
run_test "go_list_modules" "go list -m all"

log "=== Go tests completed ==="
