#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "========================================="
echo "  scproxy Integration Test Runner"
echo "========================================="

# Build scproxy
if [ ! -f "$PROJECT_ROOT/scproxy" ]; then
    echo "Building scproxy..."
    make -C "$PROJECT_ROOT" build
fi

# Cleanup previous state
echo "Cleaning previous test data..."
rm -rf "$SCRIPT_DIR/reporters/results"/*
rm -rf "$SCRIPT_DIR/cache"/*
mkdir -p "$SCRIPT_DIR/reporters/results"
mkdir -p "$SCRIPT_DIR/cache"

# Kill stale scproxy processes
pkill -f "scproxy.*--config.*integration/configs/scproxy.json" 2>/dev/null || true
sleep 1

# Start scproxy on host
echo "Starting scproxy..."
cd "$PROJECT_ROOT"
./scproxy --config tests/integration/configs/scproxy.json > tests/integration/scproxy.stdout.log 2>&1 &
PRXY_PID=$!
cd "$SCRIPT_DIR"
echo "scproxy started (PID: $PRXY_PID)"

# Wait for scproxy to bind ports
echo "Waiting for scproxy to start..."
for i in $(seq 1 15); do
    if curl -sf --connect-timeout 5 http://localhost:18800/debian/dists/bookworm/Release > /dev/null 2>&1; then
        echo "scproxy is ready (port 18800 reachable)"
        break
    fi
    sleep 1
done

if ! ps -p "$PRXY_PID" > /dev/null 2>&1; then
    echo "ERROR: scproxy crashed. Log:"
    cat "$SCRIPT_DIR/scproxy.stdout.log"
    exit 1
fi

# Run Docker test containers
echo ""
echo "Starting test containers..."
docker compose up 2>&1 || true

# Stop scproxy
echo ""
echo "Stopping scproxy..."
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
echo "  scproxy:  tests/integration/scproxy.stdout.log"
