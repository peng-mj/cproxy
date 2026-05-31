#!/bin/bash
# prxy integration test - APT mirror
# Container: debian:bookworm-slim
# prxy route: 18800 -> https://mirrors.aliyun.com
set -e

RESULT_FILE="/results/apt_results.json"
LOG_FILE="/results/apt.log"

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

# --- Setup: configure apt to use prxy proxy ---
PROXY_HOST="${PROXY_HOST:-host.docker.internal}"
PROXY_PORT="${PROXY_PORT:-18800}"
MIRROR="http://${PROXY_HOST}:${PROXY_PORT}"

log "Configuring APT mirror: $MIRROR"

cat > /etc/apt/sources.list << EOF
deb ${MIRROR}/debian/ bookworm main contrib non-free non-free-firmware
deb ${MIRROR}/debian/ bookworm-updates main contrib non-free non-free-firmware
EOF

log "Bootstrap: apt-get update + install jq"
apt-get update -y >> "$LOG_FILE" 2>&1
apt-get install -y --no-install-recommends jq >> "$LOG_FILE" 2>&1

# --- Tests ---

# Test 1: apt update (first time -> MISS)
run_test "apt_update_first" "apt-get update -y"

# Test 2: apt update (second time -> should be faster, cache HIT)
run_test "apt_update_second" "apt-get update -y"

# Test 3: install a small package
run_test "apt_install_hello" "apt-get install -y --no-install-recommends hello"

# Test 4: install another small package
run_test "apt_install_tree" "apt-get install -y --no-install-recommends tree"

# Test 5: reinstall same package (cache HIT expected for .deb)
run_test "apt_reinstall_hello" "apt-get install --reinstall -y --no-install-recommends hello"

# Test 6: apt upgrade (dry-run)
run_test "apt_upgrade_dryrun" "apt-get upgrade --dry-run -y"

# Test 7: download a package without installing
run_test "apt_download" "apt-get download curl"

log "=== APT tests completed ==="
