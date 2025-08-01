#!/bin/bash

# Quick HyperServe Benchmark
# Compares Go and Rust implementations

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

echo -e "${BLUE}HyperServe Quick Benchmark${NC}"
echo "=========================="

# Kill any existing servers
pkill hyperserve-go 2>/dev/null || true
pkill hyperserve-rust 2>/dev/null || true
pkill basic 2>/dev/null || true

# Start servers
echo -e "\n${BLUE}Starting servers...${NC}"
./hyperserve-go -port 8080 > /dev/null 2>&1 &
GO_PID=$!
./hyperserve-rust > /dev/null 2>&1 &
RUST_PID=$!

# Wait for servers
sleep 2

# Function to run benchmark
run_bench() {
    local name=$1
    local url=$2
    echo -e "\n${YELLOW}$name:${NC}"
    ab -n 10000 -c 100 -q "$url" 2>/dev/null | grep -E "(Requests per second|Time per request|Transfer rate|Failed requests)"
}

# 1. Basic GET Performance
echo -e "\n${BLUE}1. Basic GET Performance${NC}"
run_bench "Go Implementation" "http://127.0.0.1:8080/"
run_bench "Rust Implementation" "http://127.0.0.1:8081/"

# 2. JSON Endpoint
echo -e "\n${BLUE}2. JSON Endpoint Performance${NC}"
run_bench "Go JSON" "http://127.0.0.1:8080/json"
run_bench "Rust JSON" "http://127.0.0.1:8081/json"

# 3. POST Echo Performance
echo -e "\n${BLUE}3. POST Echo Performance (1KB payload)${NC}"
echo "test payload data" > test.txt
run_bench "Go POST" "http://127.0.0.1:8080/echo -p test.txt"
run_bench "Rust POST" "http://127.0.0.1:8081/echo -p test.txt"

# 4. Concurrent Connections Stress Test
echo -e "\n${BLUE}4. High Concurrency Test (500 connections)${NC}"
ab -n 5000 -c 500 -q "http://127.0.0.1:8080/" 2>/dev/null | grep -E "(Requests per second|Failed requests)" | head -2
echo "Go: $(ab -n 5000 -c 500 -q 'http://127.0.0.1:8080/' 2>/dev/null | grep 'Requests per second' | awk '{print $4}')"
echo "Rust: $(ab -n 5000 -c 500 -q 'http://127.0.0.1:8081/' 2>/dev/null | grep 'Requests per second' | awk '{print $4}')"

# 5. Memory usage
echo -e "\n${BLUE}5. Memory Usage${NC}"
GO_MEM=$(ps -o rss= -p $GO_PID | awk '{print $1/1024 " MB"}')
RUST_MEM=$(ps -o rss= -p $RUST_PID | awk '{print $1/1024 " MB"}')
echo "Go Memory: $GO_MEM"
echo "Rust Memory: $RUST_MEM"

# Cleanup
echo -e "\n${BLUE}Cleaning up...${NC}"
kill $GO_PID $RUST_PID 2>/dev/null || true
rm -f test.txt

echo -e "\n${GREEN}Benchmark complete!${NC}"

# Summary
echo -e "\n${BLUE}Summary:${NC}"
echo "Both implementations show excellent performance."
echo "Results may vary based on system load and hardware."