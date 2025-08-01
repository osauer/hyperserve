#!/bin/bash

# HyperServe Benchmark Runner
# Compares Go and Rust implementations

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
GO_PORT=8080
RUST_PORT=8081
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
    local name=$2
    local max_attempts=30
    local attempt=0
    
    echo -n "Waiting for $name server on port $port..."
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

# Function to run benchmark
run_benchmark() {
    local name=$1
    local url=$2
    local extra_args=$3
    
    echo -e "\n${BLUE}Running benchmark: $name${NC}"
    
    if command -v wrk &> /dev/null; then
        wrk -t$THREADS -c$CONNECTIONS -d${DURATION}s $extra_args "$url" | tee "$RESULTS_DIR/${name}.txt"
    elif command -v ab &> /dev/null; then
        ab -n 10000 -c $CONNECTIONS $extra_args "$url" | tee "$RESULTS_DIR/${name}.txt"
    else
        echo -e "${RED}Error: Neither wrk nor ab (Apache Bench) found. Please install one.${NC}"
        exit 1
    fi
}

# Build servers
echo -e "\n${BLUE}Building servers...${NC}"

# Build Go server
echo "Building Go server..."
cd ../go
go build -o ../benchmarks/hyperserve-go ./cmd/example-server

# Build Rust server
echo "Building Rust server..."
cd ../rust
cargo build --release --example basic
cp target/release/examples/basic ../benchmarks/hyperserve-rust

cd ../benchmarks

# Start servers
echo -e "\n${BLUE}Starting servers...${NC}"

# Start Go server
./hyperserve-go -port $GO_PORT > "$RESULTS_DIR/go_server.log" 2>&1 &
GO_PID=$!
echo "Go server PID: $GO_PID"

# Start Rust server
./hyperserve-rust > "$RESULTS_DIR/rust_server.log" 2>&1 &
RUST_PID=$!
echo "Rust server PID: $RUST_PID"

# Wait for servers to start
wait_for_server $GO_PORT "Go" || { kill $GO_PID $RUST_PID 2>/dev/null; exit 1; }
wait_for_server $RUST_PORT "Rust" || { kill $GO_PID $RUST_PID 2>/dev/null; exit 1; }

# Run benchmarks
echo -e "\n${BLUE}Running benchmarks...${NC}"

# 1. Basic HTTP GET
run_benchmark "go_http_get" "http://localhost:$GO_PORT/"
run_benchmark "rust_http_get" "http://localhost:$RUST_PORT/"

# 2. HTTP POST with small payload
echo "small payload test" > small.txt
run_benchmark "go_http_post_small" "http://localhost:$GO_PORT/echo" "-s small.txt"
run_benchmark "rust_http_post_small" "http://localhost:$RUST_PORT/echo" "-s small.txt"

# 3. HTTP POST with large payload
dd if=/dev/zero of=large.txt bs=1M count=10 2>/dev/null
run_benchmark "go_http_post_large" "http://localhost:$GO_PORT/echo" "-s large.txt"
run_benchmark "rust_http_post_large" "http://localhost:$RUST_PORT/echo" "-s large.txt"

# 4. Concurrent connections test
run_benchmark "go_concurrent" "http://localhost:$GO_PORT/" "-c 1000"
run_benchmark "rust_concurrent" "http://localhost:$RUST_PORT/" "-c 1000"

# Cleanup
echo -e "\n${BLUE}Cleaning up...${NC}"
kill $GO_PID $RUST_PID 2>/dev/null || true
rm -f small.txt large.txt hyperserve-go hyperserve-rust

# Generate summary
echo -e "\n${BLUE}Generating summary...${NC}"
python3 analyze_results.py "$RESULTS_DIR" > "$RESULTS_DIR/summary.txt"
cat "$RESULTS_DIR/summary.txt"

echo -e "\n${GREEN}Benchmark complete!${NC}"
echo "Results saved to: $RESULTS_DIR"