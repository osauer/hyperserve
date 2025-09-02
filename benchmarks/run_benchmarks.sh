#!/bin/bash

# HyperServe Benchmark Runner

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PORT=8080
DURATION=30
CONNECTIONS=100
THREADS=4

# Results directory
RESULTS_DIR="results/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo -e "${BLUE}HyperServe Benchmark Suite${NC}"
echo "=========================="
echo "Duration: ${DURATION}s"
echo "Connections: ${CONNECTIONS}"
echo "Threads: ${THREADS}"
echo ""

# Function to check if server is ready
wait_for_server() {
    local port=$1
    local max_attempts=30
    local attempt=0
    
    echo -n "Waiting for server on port $port..."
    while ! nc -z localhost $port 2>/dev/null; do
        if [ $attempt -ge $max_attempts ]; then
            echo -e " ${RED}FAILED${NC}"
            return 1
        fi
        sleep 1
        attempt=$((attempt + 1))
        echo -n "."
    done
    echo -e " ${GREEN}OK${NC}"
    return 0
}

# Build the server
echo -e "${BLUE}Building HyperServe...${NC}"
go build -o hyperserve-bench ./cmd/hyperserve

# Start the server
echo -e "${BLUE}Starting HyperServe server...${NC}"
./hyperserve-bench -port $PORT &
SERVER_PID=$!

# Wait for server to be ready
if ! wait_for_server $PORT; then
    echo -e "${RED}Failed to start server${NC}"
    exit 1
fi

# Function to run benchmark
run_benchmark() {
    local endpoint=$1
    local name=$2
    local method=${3:-GET}
    local data=${4:-}
    
    echo -e "\n${BLUE}Benchmarking: $name${NC}"
    echo "Endpoint: $endpoint"
    echo "Method: $method"
    
    if [ -n "$data" ]; then
        wrk -t$THREADS -c$CONNECTIONS -d${DURATION}s \
            --latency \
            -s <(echo "wrk.method = '$method'; wrk.body = '$data'; wrk.headers['Content-Type'] = 'application/json'") \
            "http://localhost:$PORT$endpoint" > "$RESULTS_DIR/${name}.txt" 2>&1
    else
        wrk -t$THREADS -c$CONNECTIONS -d${DURATION}s \
            --latency \
            "http://localhost:$PORT$endpoint" > "$RESULTS_DIR/${name}.txt" 2>&1
    fi
    
    # Display summary
    grep -E "Requests/sec:|Latency" "$RESULTS_DIR/${name}.txt" || true
}

# Run benchmarks
echo -e "\n${GREEN}Running benchmarks...${NC}"

run_benchmark "/" "root_endpoint"
run_benchmark "/api/health" "health_check"
run_benchmark "/api/echo" "echo_endpoint" "POST" '{"message":"test"}'

# High concurrency test
echo -e "\n${BLUE}High concurrency test (500 connections)...${NC}"
wrk -t$THREADS -c500 -d${DURATION}s \
    --latency \
    "http://localhost:$PORT/" > "$RESULTS_DIR/high_concurrency.txt" 2>&1
grep -E "Requests/sec:|Latency" "$RESULTS_DIR/high_concurrency.txt" || true

# Cleanup
echo -e "\n${BLUE}Cleaning up...${NC}"
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo -e "\n${GREEN}Benchmark complete!${NC}"
echo "Results saved to: $RESULTS_DIR"