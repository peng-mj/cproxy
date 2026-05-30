#!/bin/bash

# Automated test runner for prxy
# Runs all test scripts in the scripts directory

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PRXY_BIN="$SCRIPT_DIR/prxy"
TESTS_DIR="$SCRIPT_DIR/scripts"

# Temporary build artifacts
TEMP_PRXY_BIN="$SCRIPT_DIR/.prxy_test"
TEMP_BUILD_DIR="$SCRIPT_DIR/.test_build"
TEMP_CACHE_BASE="$SCRIPT_DIR/.test_cache"

# Test results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
FAILED_TEST_NAMES=()
CLEANUP_DONE=false

# Helper functions
log_header() {
    echo -e "${CYAN}${BOLD}========================================${NC}"
    echo -e "${CYAN}${BOLD}$1${NC}"
    echo -e "${CYAN}${BOLD}========================================${NC}"
    echo ""
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_test_start() {
    echo -e "${BOLD}Running: $1${NC}"
    echo ""
}

log_test_pass() {
    echo -e "${GREEN}${BOLD}✓ PASSED: $1${NC}"
    echo ""
}

log_test_fail() {
    echo -e "${RED}${BOLD}✗ FAILED: $1${NC}"
    echo ""
}

# Cleanup function
cleanup() {
    if [ "$CLEANUP_DONE" = true ]; then
        return
    fi

    log_info "Cleaning up any running processes..."
    pkill -f "python.*test_.*.py" 2>/dev/null || true
    pkill -f "prxy.*--port" 2>/dev/null || true
    sleep 1
}

# Cleanup temporary build artifacts
cleanup_environment() {
    log_header "Cleaning Up Test Environment"

    # Stop any running processes
    cleanup

    # Remove temporary binary
    if [ -f "$TEMP_PRXY_BIN" ]; then
        log_info "Removing temporary binary: $TEMP_PRXY_BIN"
        rm -f "$TEMP_PRXY_BIN"
        log_success "Temporary binary removed"
    fi

    # Remove temporary build directory
    if [ -d "$TEMP_BUILD_DIR" ]; then
        log_info "Removing temporary build directory: $TEMP_BUILD_DIR"
        rm -rf "$TEMP_BUILD_DIR"
        log_success "Temporary build directory removed"
    fi

    # Remove test cache directories
    if [ -d "$TEMP_CACHE_BASE" ]; then
        log_info "Removing test cache directories: $TEMP_CACHE_BASE"
        rm -rf "$TEMP_CACHE_BASE"
        log_success "Test cache directories removed"
    fi

    # Clean up /tmp/prxy_test if exists
    if [ -d "/tmp/prxy_test" ]; then
        log_info "Removing /tmp/prxy_test directory"
        rm -rf "/tmp/prxy_test"
        log_success "Temporary test directory removed"
    fi

    # Clean up test log files
    for log_file in prxy_test.log test_output.txt test_direct.txt; do
        if [ -f "$SCRIPT_DIR/$log_file" ]; then
            log_info "Removing test log file: $log_file"
            rm -f "$SCRIPT_DIR/$log_file"
        fi
    done

    log_success "Test environment cleanup completed"
    echo ""
    CLEANUP_DONE=true
}

# Build temporary binary for testing
build_temp_binary() {
    log_header "Building Test Binary"

    # Detect current system and architecture
    log_info "Detecting system and architecture..."

    DETECTED_GOOS=$(go env GOOS)
    DETECTED_GOARCH=$(go env GOARCH)
    DETECTED_GOMIPS=$(go env GOMIPS 2>/dev/null || echo "")
    DETECTED_GOARM=$(go env GOARM 2>/dev/null || echo "")

    log_success "Detected OS: $DETECTED_GOOS"
    log_success "Detected Architecture: $DETECTED_GOARCH"

    # Display additional architecture-specific info
    if [ -n "$DETECTED_GOMIPS" ]; then
        log_info "MIPS variant: $DETECTED_GOMIPS"
    fi
    if [ -n "$DETECTED_GOARM" ]; then
        log_info "ARM version: $DETECTED_GOARM"
    fi

    # Get Go version
    GO_VERSION=$(go version | awk '{print $3}')
    log_info "Go version: $GO_VERSION"

    # Get host system info
    if [ "$DETECTED_GOOS" = "linux" ]; then
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            log_info "Host system: $NAME $VERSION_ID"
        fi
    elif [ "$DETECTED_GOOS" = "darwin" ]; then
        MACOS_VERSION=$(sw_vers -productVersion 2>/dev/null || echo "unknown")
        log_info "Host system: macOS $MACOS_VERSION"
    elif [ "$DETECTED_GOOS" = "windows" ]; then
        log_info "Host system: Windows"
    fi

    echo ""
    log_info "Building temporary prxy binary for testing..."
    log_info "Output: $TEMP_PRXY_BIN"
    log_info "Target: $DETECTED_GOOS/$DETECTED_GOARCH"
    echo ""

    # Build with detected system and architecture
    # Use GOOS and GOARCH from go env to ensure binary matches current system
    BUILD_CMD="env GOOS=\"$DETECTED_GOOS\" GOARCH=\"$DETECTED_GOARCH\""

    # Add architecture-specific build flags
    if [ -n "$DETECTED_GOMIPS" ]; then
        BUILD_CMD="$BUILD_CMD GOMIPS=\"$DETECTED_GOMIPS\""
    fi
    if [ -n "$DETECTED_GOARM" ]; then
        BUILD_CMD="$BUILD_CMD GOARM=\"$DETECTED_GOARM\""
    fi

    BUILD_CMD="$BUILD_CMD go build -v -o \"$TEMP_PRXY_BIN\" ."

    # Execute build command
    eval $BUILD_CMD 2>&1 | while IFS= read -r line; do
        echo "  $line"
    done

    if [ ${PIPESTATUS[0]} -eq 0 ]; then
        log_success "Temporary binary built successfully"
    else
        log_error "Failed to build temporary binary"
        exit 1
    fi

    # Make it executable
    chmod +x "$TEMP_PRXY_BIN"

    # Display binary info
    BINARY_SIZE=$(stat -f%z "$TEMP_PRXY_BIN" 2>/dev/null || stat -c%s "$TEMP_PRXY_BIN" 2>/dev/null)
    BINARY_SIZE_MB=$(echo "scale=2; $BINARY_SIZE / 1024 / 1024" | bc 2>/dev/null || echo "?")
    log_info "Binary size: $BINARY_SIZE bytes (~${BINARY_SIZE_MB} MB)"

    # Display binary type information
    if command -v file &> /dev/null; then
        BINARY_TYPE=$(file "$TEMP_PRXY_BIN")
        log_info "Binary type: $BINARY_TYPE"
    fi

    # Verify binary
    if [ -f "$TEMP_PRXY_BIN" ] && [ -x "$TEMP_PRXY_BIN" ]; then
        log_success "Binary verified and executable"
    else
        log_error "Binary verification failed"
        exit 1
    fi

    echo ""
}

# Set trap for cleanup
trap cleanup EXIT INT TERM
trap cleanup_environment EXIT INT TERM

# Check prerequisites
check_prerequisites() {
    log_header "Checking Prerequisites"

    # Check Go installation
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        log_info "Please install Go from https://go.dev/dl/"
        exit 1
    fi

    GO_VERSION=$(go version | awk '{print $3}')
    log_success "Go version: $GO_VERSION"

    # Check Python version
    if ! command -v python3 &> /dev/null; then
        log_error "Python 3 is not installed"
        exit 1
    fi

    PYTHON_VERSION=$(python3 --version | awk '{print $2}')
    log_success "Python version: $PYTHON_VERSION"

    # Check if scripts directory exists
    if [ ! -d "$TESTS_DIR" ]; then
        log_error "Scripts directory not found: $TESTS_DIR"
        exit 1
    fi

    log_success "Scripts directory found: $TESTS_DIR"
    echo ""
}

# Run a single test
run_test() {
    local test_name=$1
    local test_script=$2
    local test_description=$3

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    log_header "Test $TOTAL_TESTS: $test_name"
    log_info "Description: $test_description"
    log_info "Script: $test_script"
    log_info "Using binary: $TEMP_PRXY_BIN"
    echo ""

    log_test_start "$test_name"

    # Run the test with temporary binary path
    export PRXY_BIN_PATH="$TEMP_PRXY_BIN"
    export TEST_CACHE_DIR="$TEMP_CACHE_BASE"

    if bash -c "cd '$SCRIPT_DIR' && python3 '$test_script'"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        log_test_pass "$test_name"
        return 0
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        FAILED_TEST_NAMES+=("$test_name")
        log_test_fail "$test_name"
        return 1
    fi
}

# Main test execution
main() {
    log_header "Prxy Automated Test Suite"
    echo ""
    log_info "Starting automated test execution..."
    log_info "Working directory: $SCRIPT_DIR"
    log_info "Test scripts directory: $TESTS_DIR"
    echo ""

    # Check prerequisites
    check_prerequisites

    # Build temporary binary for testing
    build_temp_binary

    # Array of tests to run
    declare -a TESTS=(
        "Cache Functionality|scripts/test_cache.py|Integration tests for HTTP cache functionality"
        "Diverse Cache Test|scripts/test_diverse_cache.py|Tests with diverse file types, sizes, and sources"
        "Batch Proxy Demo|scripts/test_batch_demo.py|Demo script for batch proxy configuration mode"
        "GitHub Releases|scripts/test_github_releases.py|Tests for GitHub releases optimization"
    )

    # Run all tests
    log_header "Running Tests"
    echo ""

    START_TIME=$(date +%s)

    for test_info in "${TESTS[@]}"; do
        IFS='|' read -r test_name test_script test_description <<< "$test_info"

        # Skip test_batch_demo.py if test-config.json doesn't exist
        if [[ "$test_script" == *"test_batch_demo.py"* ]]; then
            if [ ! -f "$SCRIPT_DIR/test-config.json" ]; then
                log_warning "Skipping $test_name (test-config.json not found)"
                echo ""
                continue
            fi
        fi

        run_test "$test_name" "$test_script" "$test_description"

        # Wait a bit between tests to ensure cleanup
        sleep 2
    done

    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    # Print summary
    log_header "Test Summary"
    echo ""

    echo -e "${BOLD}Total Tests Run:${NC} $TOTAL_TESTS"
    echo -e "${GREEN}${BOLD}Passed:${NC} $PASSED_TESTS"
    echo -e "${RED}${BOLD}Failed:${NC} $FAILED_TESTS"
    echo -e "${BOLD}Duration:${NC} ${DURATION}s"
    echo ""

    # List failed tests
    if [ $FAILED_TESTS -gt 0 ]; then
        echo -e "${RED}${BOLD}Failed Tests:${NC}"
        for test_name in "${FAILED_TEST_NAMES[@]}"; do
            echo -e "  ${RED}✗${NC} $test_name"
        done
        echo ""
    fi

    # Final result
    if [ $FAILED_TESTS -eq 0 ]; then
        log_success "All tests passed!"
        echo ""
        echo -e "${GREEN}${BOLD}╔═══════════════════════════════════════╗${NC}"
        echo -e "${GREEN}${BOLD}║                                       ║${NC}"
        echo -e "${GREEN}${BOLD}║   ✓ ALL TESTS PASSED SUCCESSFULLY!   ║${NC}"
        echo -e "${GREEN}${BOLD}║                                       ║${NC}"
        echo -e "${GREEN}${BOLD}╚═══════════════════════════════════════╝${NC}"
        echo ""
        exit 0
    else
        log_error "Some tests failed!"
        echo ""
        echo -e "${RED}${BOLD}╔═══════════════════════════════════════╗${NC}"
        echo -e "${RED}${BOLD}║                                       ║${NC}"
        echo -e "${RED}${BOLD}║   ✗ TESTS FAILED                     ║${NC}"
        echo -e "${RED}${BOLD}║                                       ║${NC}"
        echo -e "${RED}${BOLD}╚═══════════════════════════════════════╝${NC}"
        echo ""
        exit 1
    fi
}

# Run main function
main "$@"