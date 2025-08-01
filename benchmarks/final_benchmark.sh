#!/bin/bash

# Final HyperServe Benchmark
# Clean comparison between Go and Rust

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

echo -e "${BLUE}HyperServe Final Performance Benchmark${NC}"
echo "======================================"
echo "Date: $(date)"
echo ""

# Kill any existing servers
pkill hyperserve 2>/dev/null || true
pkill basic 2>/dev/null || true
sleep 1

# Start servers
echo -e "${BLUE}Starting servers...${NC}"
./hyperserve-go -port 8080 > go.log 2>&1 &
GO_PID=$!
./hyperserve-rust-clean > rust.log 2>&1 &
RUST_PID=$!

# Wait for servers
sleep 3

# Verify servers are running
if ! curl -s http://127.0.0.1:8080/ > /dev/null; then
    echo "Go server failed to start"
    exit 1
fi

if ! curl -s http://127.0.0.1:8081/ > /dev/null; then
    echo "Rust server failed to start"
    exit 1
fi

echo "Servers running - Go: $GO_PID, Rust: $RUST_PID"
echo ""

# Function to run benchmark and extract key metrics
benchmark() {
    local name=$1
    local url=$2
    local connections=$3
    
    echo -e "${YELLOW}$name:${NC}"
    result=$(ab -n 10000 -c $connections -q "$url" 2>/dev/null)
    
    rps=$(echo "$result" | grep "Requests per second" | awk '{print $4}')
    lat_mean=$(echo "$result" | grep "Time per request" | head -1 | awk '{print $4}')
    failed=$(echo "$result" | grep "Failed requests" | awk '{print $3}')
    
    echo "  Requests/sec: $rps"
    echo "  Latency (ms): $lat_mean"
    echo "  Failed: $failed"
    echo ""
}

echo -e "${BLUE}1. Single-threaded Performance (1 connection)${NC}"
benchmark "Go" "http://127.0.0.1:8080/" 1
benchmark "Rust" "http://127.0.0.1:8081/" 1

echo -e "${BLUE}2. Normal Load (100 connections)${NC}"
benchmark "Go" "http://127.0.0.1:8080/" 100
benchmark "Rust" "http://127.0.0.1:8081/" 100

echo -e "${BLUE}3. High Concurrency (500 connections)${NC}"
benchmark "Go" "http://127.0.0.1:8080/" 500
benchmark "Rust" "http://127.0.0.1:8081/" 500

echo -e "${BLUE}4. JSON Endpoint Performance${NC}"
benchmark "Go JSON" "http://127.0.0.1:8080/json" 100
benchmark "Rust JSON" "http://127.0.0.1:8081/json" 100

echo -e "${BLUE}5. Memory Usage${NC}"
GO_MEM=$(ps -o rss= -p $GO_PID 2>/dev/null | awk '{print $1/1024 " MB"}' || echo "N/A")
RUST_MEM=$(ps -o rss= -p $RUST_PID 2>/dev/null | awk '{print $1/1024 " MB"}' || echo "N/A")
echo "Go Memory: $GO_MEM"
echo "Rust Memory: $RUST_MEM"

# Cleanup
echo -e "\n${BLUE}Cleaning up...${NC}"
kill $GO_PID $RUST_PID 2>/dev/null || true

echo -e "\n${GREEN}Benchmark complete!${NC}"