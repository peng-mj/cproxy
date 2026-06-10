#!/bin/bash
# scproxy integration test - npm
# Container: node:20-slim
# scproxy route: 18800 -> https://mirrors.aliyun.com
# Using npmmirror via aliyun proxy
set -e

RESULT_FILE="/results/npm_results.json"
LOG_FILE="/results/npm.log"

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
NPM_PORT="${NPM_PORT:-18802}"
NPM_REGISTRY="http://${PROXY_HOST}:${NPM_PORT}"

log "npm registry: $NPM_REGISTRY"

# Install jq
apt-get update -y > /dev/null 2>&1 && apt-get install -y --no-install-recommends jq > /dev/null 2>&1 || true

# Create test project
mkdir -p /tmp/npmtest
cd /tmp/npmtest

# --- Tests ---

# Test 1: init project
run_test "npm_init" "npm init -y"

# Test 2: set registry
run_test "npm_set_registry" "npm config set registry $NPM_REGISTRY"

# Test 3: install a simple package (first time)
run_test "npm_install_lodash_first" "npm install lodash@4.17.21 --no-package-lock"

# Test 4: install another package
run_test "npm_install_express" "npm install express@4.21.0 --no-package-lock"

# Test 5: reinstall lodash (cache HIT expected)
run_test "npm_install_lodash_second" "npm install lodash@4.17.21 --no-package-lock"

# Test 6: install from package.json
cat > package.json << 'EOF'
{
  "name": "scproxy-test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "4.17.21",
    "axios": "1.7.9"
  }
}
EOF
rm -rf node_modules package-lock.json
run_test "npm_install_clean" "npm install"

# Test 7: list packages
run_test "npm_list" "npm list --depth=0"

log "=== NPM tests completed ==="
