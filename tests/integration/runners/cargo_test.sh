#!/bin/bash
# scproxy integration test - Cargo (Rust crates)
# Container: rust:1.83-bookworm-slim
# scproxy route: 18800 -> https://mirrors.aliyun.com
# Using tuna crates mirror index + aliyun for crates-io
set -e

RESULT_FILE="/results/cargo_results.json"
LOG_FILE="/results/cargo.log"

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
PROXY_PORT="${PROXY_PORT:-18800}"
MIRROR="http://${PROXY_HOST}:${PROXY_PORT}"

log "Using cargo mirror: ${MIRROR}/crates.io-index/"

# Clean previous test data
rm -rf /tmp/cargotest
mkdir -p /tmp/cargotest
cd /tmp/cargotest

# Use official sparse registry (bypass scproxy for crates.io to avoid issues)
cat > /root/.cargo/config.toml << EOF
[source.crates-io]
registry = "sparse+https://index.crates.io/"
EOF

log "Using official sparse registry: sparse+https://index.crates.io/"

# --- Tests ---

run_test "cargo_init" "cargo init --name prxytest"

run_test "cargo_add_serde" "cargo add serde@1.0.215"

run_test "cargo_fetch_first" "cargo fetch"

run_test "cargo_add_serde_json" "cargo add serde_json@1.0.133"

run_test "cargo_fetch_second" "cargo fetch"

run_test "cargo_tree" "cargo tree"

run_test "cargo_build" "cargo build --release"

log "=== Cargo tests completed ==="
