#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Cleaning up..."

docker compose -f "$SCRIPT_DIR/docker-compose.yml" down --remove-orphans 2>/dev/null || true
pkill -f "scproxy.*--config.*integration/configs/scproxy.json" 2>/dev/null || true

rm -rf "$SCRIPT_DIR/cache"/*
rm -rf "$SCRIPT_DIR/reporters/results"/*
rm -f "$SCRIPT_DIR/scproxy.stdout.log" "$SCRIPT_DIR/scproxy.log"

echo "Done."
