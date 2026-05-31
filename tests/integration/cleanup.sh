#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Cleaning up..."

docker compose -f "$SCRIPT_DIR/docker-compose.yml" down --remove-orphans 2>/dev/null || true
pkill -f "prxy.*--config.*integration/configs/prxy.json" 2>/dev/null || true

rm -rf "$SCRIPT_DIR/cache"/*
rm -rf "$SCRIPT_DIR/reporters/results"/*
rm -f "$SCRIPT_DIR/prxy.stdout.log" "$SCRIPT_DIR/prxy.log"

echo "Done."
