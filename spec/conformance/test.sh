#!/bin/bash
# HyperServe Conformance Test Suite

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test configuration
PORT=8080
TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$(dirname "$TEST_DIR")")"

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "PASS") echo -e "${GREEN}[PASS]${NC} $message" ;;
        "FAIL") echo -e "${RED}[FAIL]${NC} $message" ;;
        "INFO") echo -e "${YELLOW}[INFO]${NC} $message" ;;
    esac
}

# Start server function
start_server() {
    local pid_file="/tmp/hyperserve.pid"
    
    print_status "INFO" "Starting HyperServe server on port $PORT..."
    
    (cd "$ROOT_DIR" && go run ./cmd/hyperserve -port $PORT > /tmp/hyperserve.log 2>&1) &
    
    echo $! > $pid_file
    sleep 2  # Give server time to start
    
    # Check if server started
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        print_status "PASS" "Server started successfully"
        return 0
    else
        print_status "FAIL" "Server failed to start"
        cat /tmp/hyperserve.log
        return 1
    fi
}

# Stop server function
stop_server() {
    local pid_file="/tmp/hyperserve.pid"
    
    if [ -f $pid_file ]; then
        kill $(cat $pid_file) 2>/dev/null || true
        rm $pid_file
        print_status "INFO" "Server stopped"
    fi
}

# Test function
run_test() {
    local test_name=$1
    local endpoint=$2
    local expected_status=$3
    local method=${4:-GET}
    local data=${5:-}
    
    local response
    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X $method -d "$data" -H "Content-Type: application/json" http://localhost:$PORT$endpoint)
    else
        response=$(curl -s -w "\n%{http_code}" -X $method http://localhost:$PORT$endpoint)
    fi
    
    local status_code=$(echo "$response" | tail -n1)
    
    if [ "$status_code" = "$expected_status" ]; then
        print_status "PASS" "$test_name"
        return 0
    else
        print_status "FAIL" "$test_name (expected $expected_status, got $status_code)"
        return 1
    fi
}

# Cleanup on exit
trap stop_server EXIT

# Main test execution
main() {
    print_status "INFO" "Starting HyperServe Conformance Tests"
    
    # Start the server
    if ! start_server; then
        exit 1
    fi
    
    # Run tests
    local total_tests=0
    local passed_tests=0
    
    # Basic endpoint tests
    run_test "Health Check" "/health" "200" && ((passed_tests++)) || true
    ((total_tests++))
    
    run_test "Root Endpoint" "/" "200" && ((passed_tests++)) || true
    ((total_tests++))
    
    run_test "404 Not Found" "/nonexistent" "404" && ((passed_tests++)) || true
    ((total_tests++))
    
    # API tests
    run_test "POST Echo" "/api/echo" "200" "POST" '{"message":"test"}' && ((passed_tests++)) || true
    ((total_tests++))
    
    # MCP tests if enabled
    if [ "${MCP_ENABLED:-false}" = "true" ]; then
        run_test "MCP Discovery" "/.well-known/mcp.json" "200" && ((passed_tests++)) || true
        ((total_tests++))
    fi
    
    # Print summary
    echo ""
    print_status "INFO" "Test Summary: $passed_tests/$total_tests passed"
    
    if [ $passed_tests -eq $total_tests ]; then
        print_status "PASS" "All tests passed!"
        exit 0
    else
        print_status "FAIL" "Some tests failed"
        exit 1
    fi
}

# Run main function
main "$@"