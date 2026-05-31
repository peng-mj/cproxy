#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "========================================="
echo "  prxy Integration Test Runner"
echo "========================================="

# Build prxy
if [ ! -f "$PROJECT_ROOT/prxy" ]; then
    echo "Building prxy..."
    make -C "$PROJECT_ROOT" build
fi

# Cleanup previous state
echo "Cleaning previous test data..."
rm -rf "$SCRIPT_DIR/reporters/results"/*
rm -rf "$SCRIPT_DIR/cache"/*
mkdir -p "$SCRIPT_DIR/reporters/results"
mkdir -p "$SCRIPT_DIR/cache"

# Kill stale prxy processes
pkill -f "prxy.*--config.*integration/configs/prxy.json" 2>/dev/null || true
sleep 1

# Start prxy on host
echo "Starting prxy..."
cd "$PROJECT_ROOT"
./prxy --config tests/integration/configs/prxy.json > tests/integration/prxy.stdout.log 2>&1 &
PRXY_PID=$!
cd "$SCRIPT_DIR"
echo "prxy started (PID: $PRXY_PID)"

# Wait for prxy to bind ports
echo "Waiting for prxy to start..."
for i in $(seq 1 15); do
    if curl -sf --connect-timeout 5 http://localhost:18800/debian/dists/bookworm/Release > /dev/null 2>&1; then
        echo "prxy is ready (port 18800 reachable)"
        break
    fi
    sleep 1
done

if ! ps -p "$PRXY_PID" > /dev/null 2>&1; then
    echo "ERROR: prxy crashed. Log:"
    cat "$SCRIPT_DIR/prxy.stdout.log"
    exit 1
fi

# Run Docker test containers
echo ""
echo "Starting test containers..."
docker compose up 2>&1 || true

# Stop prxy
echo ""
echo "Stopping prxy..."
kill "$PRXY_PID" 2>/dev/null || true
wait "$PRXY_PID" 2>/dev/null || true

# Generate report on host
echo ""
echo "Generating report..."
cd "$SCRIPT_DIR"
RESULTS_DIR="$SCRIPT_DIR/reporters/results" python3 "$SCRIPT_DIR/reporters/generate_report.py"

echo ""
echo "========================================="
echo "  Test Complete"
echo "========================================="

if [ -f "$SCRIPT_DIR/reporters/results/report.json" ]; then
    python3 -c "
import json
r = json.load(open('$SCRIPT_DIR/reporters/results/report.json'))
s = r['summary']
print(f\"  Passed:  {s['passed']}\")
print(f\"  Failed:  {s['failed']}\")
print(f\"  Duration: {s['duration']}s\")
"
else
    echo "  WARNING: No report generated"
fi

echo ""
echo "Reports:"
echo "  HTML:  tests/integration/reporters/results/report.html"
echo "  MD:    tests/integration/reporters/results/report.md"
echo "  JSON:  tests/integration/reporters/results/report.json"
echo "  prxy:  tests/integration/prxy.stdout.log"
