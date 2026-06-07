#!/bin/bash
# prxy service management script for Linux
# Usage: ./prxy-service.sh {start|stop|status|restart|logs}

set -e

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Configuration
PRXY_BIN="$SCRIPT_DIR/prxy"
PID_FILE="$SCRIPT_DIR/prxy.pid"
LOG_FILE="$SCRIPT_DIR/prxy.log"
DEFAULT_CONFIG="$SCRIPT_DIR/cache/config.json"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print colored messages
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check if prxy binary exists
check_binary() {
    if [ ! -f "$PRXY_BIN" ]; then
        print_error "prxy binary not found at: $PRXY_BIN"
        print_info "Please build prxy first: make build"
        exit 1
    fi
    if [ ! -x "$PRXY_BIN" ]; then
        print_warn "Making prxy binary executable..."
        chmod +x "$PRXY_BIN"
    fi
}

# Check if process is running
is_running() {
    if [ ! -f "$PID_FILE" ]; then
        return 1
    fi

    local pid=$(cat "$PID_FILE")
    if [ -z "$pid" ]; then
        return 1
    fi

    # Check if process exists
    if kill -0 "$pid" 2>/dev/null; then
        return 0
    else
        # Stale PID file
        rm -f "$PID_FILE"
        return 1
    fi
}

# Get process info
get_process_info() {
    if [ -f "$PID_FILE" ]; then
        local pid=$(cat "$PID_FILE")
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            echo "$pid"
            return 0
        fi
    fi
    return 1
}

# Start service
start_service() {
    print_info "Starting prxy service..."

    if is_running; then
        local pid=$(cat "$PID_FILE")
        print_warn "prxy is already running (PID: $pid)"
        print_info "Use 'status' to see details or 'stop' to stop it first"
        exit 0
    fi

    check_binary

    # Check if config exists
    if [ ! -f "$DEFAULT_CONFIG" ]; then
        print_warn "Config file not found at: $DEFAULT_CONFIG"
        print_info "prxy will create a default config on first run"
    fi

    # Start prxy in background
    print_info "Using config: $DEFAULT_CONFIG"
    print_info "Logging to: $LOG_FILE"

    nohup "$PRXY_BIN" --config "$DEFAULT_CONFIG" >> "$LOG_FILE" 2>&1 &
    local pid=$!

    # Save PID
    echo "$pid" > "$PID_FILE"

    # Wait a moment and check if process started successfully
    sleep 1
    if kill -0 "$pid" 2>/dev/null; then
        print_info "prxy started successfully (PID: $pid)"
        print_info "PID file: $PID_FILE"
        print_info "Log file: $LOG_FILE"
    else
        print_error "Failed to start prxy. Check log file: $LOG_FILE"
        rm -f "$PID_FILE"
        exit 1
    fi
}

# Stop service
stop_service() {
    print_info "Stopping prxy service..."

    if ! is_running; then
        print_warn "prxy is not running"
        if [ -f "$PID_FILE" ]; then
            rm -f "$PID_FILE"
        fi
        exit 0
    fi

    local pid=$(cat "$PID_FILE")
    print_info "Sending SIGTERM to process $pid..."

    # Try graceful shutdown first
    kill "$pid" 2>/dev/null || true

    # Wait for process to terminate (max 10 seconds)
    local count=0
    while kill -0 "$pid" 2>/dev/null && [ $count -lt 10 ]; do
        sleep 1
        count=$((count + 1))
    done

    # Check if still running
    if kill -0 "$pid" 2>/dev/null; then
        print_warn "Process did not stop gracefully, forcing..."
        kill -9 "$pid" 2>/dev/null || true
        sleep 1
    fi

    # Clean up PID file
    rm -f "$PID_FILE"

    # Final check
    if kill -0 "$pid" 2>/dev/null; then
        print_error "Failed to stop prxy (PID: $pid)"
        exit 1
    else
        print_info "prxy stopped successfully"
    fi
}

# Show service status
show_status() {
    print_info "prxy service status:"

    if is_running; then
        local pid=$(cat "$PID_FILE")
        echo -e "  Status: ${GREEN}Running${NC}"
        echo "  PID: $pid"
        echo "  PID File: $PID_FILE"
        echo "  Log File: $LOG_FILE"
        echo "  Binary: $PRXY_BIN"

        # Show uptime and resource usage if available
        if command -v ps >/dev/null 2>&1; then
            echo ""
            print_info "Process details:"
            ps -p "$pid" -o pid,etime,pcpu,pmem,cmd 2>/dev/null || true
        fi

        # Show recent log entries
        if [ -f "$LOG_FILE" ] && [ -s "$LOG_FILE" ]; then
            echo ""
            print_info "Recent log entries (last 5 lines):"
            tail -5 "$LOG_FILE" 2>/dev/null | sed 's/^/  /' || true
        fi
    else
        echo -e "  Status: ${YELLOW}Stopped${NC}"
        if [ -f "$PID_FILE" ]; then
            print_warn "Removing stale PID file"
            rm -f "$PID_FILE"
        fi
    fi

    # Show config info
    echo ""
    print_info "Configuration:"
    if [ -f "$DEFAULT_CONFIG" ]; then
        echo "  Config file: $DEFAULT_CONFIG (exists)"
        if command -v jq >/dev/null 2>&1; then
            echo "  Cache directory: $(jq -r '.cache.directory // "not set"' "$DEFAULT_CONFIG" 2>/dev/null || echo "unknown")"
        fi
    else
        echo "  Config file: $DEFAULT_CONFIG (not found - will be created on first start)"
    fi
}

# Show/monitor logs
show_logs() {
    if [ ! -f "$LOG_FILE" ]; then
        print_warn "Log file does not exist: $LOG_FILE"
        print_info "The log file will be created when prxy starts"
        exit 1
    fi

    print_info "Monitoring log file: $LOG_FILE"
    print_info "Press Ctrl+C to stop monitoring"
    echo ""

    # Follow the log file
    tail -f "$LOG_FILE"
}

# Main
case "${1:-}" in
    start)
        start_service
        ;;
    stop)
        stop_service
        ;;
    status)
        show_status
        ;;
    restart)
        stop_service
        sleep 1
        start_service
        ;;
    logs)
        show_logs
        ;;
    *)
        echo "prxy service management script"
        echo ""
        echo "Usage: $0 {start|stop|status|restart|logs}"
        echo ""
        echo "Commands:"
        echo "  start   - Start prxy service"
        echo "  stop    - Stop prxy service"
        echo "  status  - Show service status"
        echo "  restart - Restart service"
        echo "  logs    - Monitor log file in real-time (tail -f)"
        echo ""
        echo "Files:"
        echo "  PID file:  $PID_FILE"
        echo "  Log file:  $LOG_FILE"
        echo "  Config:    $DEFAULT_CONFIG"
        exit 1
        ;;
esac
