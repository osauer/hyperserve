#!/bin/bash
# HyperServe Conformance Test Suite
# Tests both Go and Rust implementations for feature parity

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test configuration
GO_PORT=8080
RUST_PORT=8081
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
    local impl=$1
    local port=$2
    local pid_file="/tmp/hyperserve_${impl}.pid"
    
    print_status "INFO" "Starting $impl server on port $port..."
    
    if [ "$impl" = "go" ]; then
        (cd "$ROOT_DIR/go" && go run ./cmd/server -port $port > /tmp/hyperserve_${impl}.log 2>&1) &
    else
        (cd "$ROOT_DIR/rust" && cargo run --release --features mcp --bin hyperserve-server -- --port $port > /tmp/hyperserve_${impl}.log 2>&1) &
    fi
    
    echo $! > $pid_file
    sleep 2  # Give server time to start
    
    # Check if server started
    if curl -s http://localhost:$port/health > /dev/null 2>&1; then
        print_status "PASS" "$impl server started successfully"
        return 0
    else
        print_status "FAIL" "$impl server failed to start"
        cat /tmp/hyperserve_${impl}.log
        return 1
    fi
}

# Stop server function
stop_server() {
    local impl=$1
    local pid_file="/tmp/hyperserve_${impl}.pid"
    
    if [ -f $pid_file ]; then
        kill $(cat $pid_file) 2>/dev/null || true
        rm -f $pid_file
    fi
}

# Run test function
run_test() {
    local test_name=$1
    local port=$2
    local endpoint=$3
    local method=$4
    local data=$5
    local expected_status=$6
    
    local response_file="/tmp/response_${port}.json"
    local headers_file="/tmp/headers_${port}.txt"
    
    # Make request
    if [ "$method" = "GET" ]; then
        curl -s -w "\n%{http_code}" -D $headers_file \
            http://localhost:$port$endpoint \
            > $response_file 2>/dev/null
    else
        curl -s -w "\n%{http_code}" -D $headers_file \
            -X $method \
            -H "Content-Type: application/json" \
            -d "$data" \
            http://localhost:$port$endpoint \
            > $response_file 2>/dev/null
    fi
    
    # Extract status code (last line)
    local status_code=$(tail -n1 $response_file)
    local body=$(head -n-1 $response_file)
    
    # Check status code
    if [ "$status_code" = "$expected_status" ]; then
        echo "$body" > $response_file
        return 0
    else
        print_status "FAIL" "$test_name: Expected status $expected_status, got $status_code"
        return 1
    fi
}

# Compare responses function
compare_responses() {
    local test_name=$1
    local go_response="/tmp/response_${GO_PORT}.json"
    local rust_response="/tmp/response_${RUST_PORT}.json"
    
    # For JSON responses, use jq to normalize before comparing
    if command -v jq > /dev/null 2>&1; then
        if [ -s "$go_response" ] && [ -s "$rust_response" ]; then
            # Try to parse as JSON
            if jq . "$go_response" > /tmp/go_normalized.json 2>/dev/null && \
               jq . "$rust_response" > /tmp/rust_normalized.json 2>/dev/null; then
                if diff -q /tmp/go_normalized.json /tmp/rust_normalized.json > /dev/null; then
                    print_status "PASS" "$test_name: Responses match"
                    return 0
                fi
            fi
        fi
    fi
    
    # Fallback to direct comparison
    if diff -q "$go_response" "$rust_response" > /dev/null; then
        print_status "PASS" "$test_name: Responses match"
        return 0
    else
        print_status "FAIL" "$test_name: Responses differ"
        echo "Go response:"
        cat "$go_response"
        echo -e "\nRust response:"
        cat "$rust_response"
        return 1
    fi
}

# Test basic HTTP endpoints
test_basic_http() {
    local port=$1
    local impl=$2
    
    print_status "INFO" "Testing basic HTTP endpoints for $impl..."
    
    # Test health endpoint
    run_test "Health Check" $port "/health" "GET" "" "200"
    
    # Test root endpoint  
    run_test "Root Path" $port "/" "GET" "" "200"
    
    # Test 404
    run_test "Not Found" $port "/nonexistent" "GET" "" "404"
    
    # Test method not allowed (accept either 404 or 405)
    if run_test "Method Not Allowed" $port "/health" "POST" '{}' "405" 2>/dev/null || \
       run_test "Method Not Allowed" $port "/health" "POST" '{}' "404" 2>/dev/null; then
        print_status "PASS" "Method Not Allowed: Server correctly rejects POST to /health"
    else
        print_status "FAIL" "Method Not Allowed: Unexpected response"
    fi
}

# Test MCP protocol
test_mcp() {
    local port=$1
    local impl=$2
    
    print_status "INFO" "Testing MCP protocol for $impl..."
    
    # Skip if MCP not enabled
    if ! curl -s http://localhost:$port/mcp > /dev/null 2>&1; then
        print_status "INFO" "MCP not enabled for $impl, skipping..."
        return 0
    fi
    
    # Test initialize
    run_test "MCP Initialize" $port "/mcp" "POST" '{
        "jsonrpc": "2.0",
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "conformance", "version": "1.0"}
        },
        "id": 1
    }' "200"
    
    # Test tools/list
    run_test "MCP List Tools" $port "/mcp" "POST" '{
        "jsonrpc": "2.0",
        "method": "tools/list",
        "id": 2
    }' "200"
}

# Test WebSocket
test_websocket() {
    local port=$1
    local impl=$2
    
    print_status "INFO" "Testing WebSocket for $impl..."
    
    # Basic WebSocket connection test using curl
    if curl -s -N \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Version: 13" \
        -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
        http://localhost:$port/ws > /tmp/ws_response_$port.txt 2>&1; then
        
        # Check for switching protocols response
        if grep -q "101 Switching Protocols" /tmp/ws_response_$port.txt; then
            print_status "PASS" "WebSocket upgrade successful"
        else
            print_status "FAIL" "WebSocket upgrade failed"
        fi
    fi
}

# Main test execution
main() {
    local mode=$1
    
    if [ -z "$mode" ]; then
        echo "Usage: $0 [go|rust|both]"
        exit 1
    fi
    
    # Clean up any existing servers
    stop_server go
    stop_server rust
    
    case $mode in
        "go")
            start_server go $GO_PORT || exit 1
            test_basic_http $GO_PORT go
            test_mcp $GO_PORT go
            test_websocket $GO_PORT go
            stop_server go
            ;;
            
        "rust")
            start_server rust $RUST_PORT || exit 1
            test_basic_http $RUST_PORT rust
            test_mcp $RUST_PORT rust  
            test_websocket $RUST_PORT rust
            stop_server rust
            ;;
            
        "both")
            # Start both servers
            start_server go $GO_PORT || exit 1
            start_server rust $RUST_PORT || exit 1
            
            print_status "INFO" "Running conformance tests..."
            
            # Run same tests on both
            local tests_passed=0
            local tests_failed=0
            
            # Basic HTTP tests
            for endpoint in "/health" "/" ; do
                run_test "GET $endpoint" $GO_PORT "$endpoint" "GET" "" "200"
                run_test "GET $endpoint" $RUST_PORT "$endpoint" "GET" "" "200"
                if compare_responses "GET $endpoint"; then
                    ((tests_passed++))
                else
                    ((tests_failed++))
                fi
            done
            
            # MCP tests (if available)
            if curl -s http://localhost:$GO_PORT/mcp > /dev/null 2>&1 && \
               curl -s http://localhost:$RUST_PORT/mcp > /dev/null 2>&1; then
                
                # Initialize
                local init_data='{
                    "jsonrpc": "2.0",
                    "method": "initialize",
                    "params": {
                        "protocolVersion": "2024-11-05",
                        "capabilities": {},
                        "clientInfo": {"name": "conformance", "version": "1.0"}
                    },
                    "id": 1
                }'
                
                run_test "MCP Initialize" $GO_PORT "/mcp" "POST" "$init_data" "200"
                run_test "MCP Initialize" $RUST_PORT "/mcp" "POST" "$init_data" "200"
                if compare_responses "MCP Initialize"; then
                    ((tests_passed++))
                else
                    ((tests_failed++))
                fi
            fi
            
            # Stop servers
            stop_server go
            stop_server rust
            
            # Summary
            echo ""
            print_status "INFO" "Conformance Test Summary"
            print_status "INFO" "Tests Passed: $tests_passed"
            if [ $tests_failed -gt 0 ]; then
                print_status "FAIL" "Tests Failed: $tests_failed"
                exit 1
            else
                print_status "PASS" "All tests passed!"
            fi
            ;;
            
        *)
            echo "Invalid mode: $mode"
            echo "Usage: $0 [go|rust|both]"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"