#!/bin/bash
# scproxy integration test - pip (PyPI)
# Container: python:3.12-slim
# scproxy route: 18800 -> https://mirrors.aliyun.com
# Using tuna pypi mirror via aliyun proxy path
set -e

RESULT_FILE="/results/pip_results.json"
LOG_FILE="/results/pip.log"

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

# Use aliyun pypi mirror through scproxy
PIP_INDEX="http://${PROXY_HOST}:${PROXY_PORT}/pypi/simple"
PIP_TRUSTED="${PROXY_HOST}:${PROXY_PORT}"

log "Using pip index: $PIP_INDEX"

# Install jq (some images don't have it)
pip install --quiet jq 2>/dev/null || true
apt-get update -y > /dev/null 2>&1 && apt-get install -y jq > /dev/null 2>&1 || true

# --- Tests ---

# Test 1: install a simple pure-python package (first time)
run_test "pip_install_requests_first" "pip install --no-cache-dir --index-url $PIP_INDEX --trusted-host $PIP_TRUSTED requests==2.32.3"

# Test 2: reinstall same package (should hit cache)
run_test "pip_install_requests_second" "pip install --no-cache-dir --index-url $PIP_INDEX --trusted-host $PIP_TRUSTED requests==2.32.3"

# Test 3: install a package with dependencies
run_test "pip_install_flask" "pip install --no-cache-dir --index-url $PIP_INDEX --trusted-host $PIP_TRUSTED flask==3.1.0"

# Test 4: install from requirements.txt
cat > /tmp/requirements.txt << 'EOF'
click==8.1.7
itsdangerous==2.2.0
EOF
run_test "pip_install_requirements" "pip install --no-cache-dir --index-url $PIP_INDEX --trusted-host $PIP_TRUSTED -r /tmp/requirements.txt"

# Test 5: download wheel without installing
mkdir -p /tmp/wheels
run_test "pip_download_wheel" "pip download --dest /tmp/wheels --index-url $PIP_INDEX --trusted-host $PIP_TRUSTED urllib3==2.2.2"

log "=== PIP tests completed ==="
