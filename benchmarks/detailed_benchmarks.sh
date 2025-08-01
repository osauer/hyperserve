#!/bin/bash

# Detailed HyperServe Benchmarks
# Tests specific aspects of both implementations

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

# Configuration
GO_PORT=8080
RUST_PORT=8081
MCP_GO_PORT=8082
MCP_RUST_PORT=8083

# Results directory
RESULTS_DIR="results/detailed_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo -e "${BLUE}HyperServe Detailed Benchmark Suite${NC}"
echo "===================================="

# Function to measure startup time
measure_startup_time() {
    local cmd=$1
    local name=$2
    local port=$3
    
    echo -e "\n${YELLOW}Measuring $name startup time...${NC}"
    
    local start_time=$(date +%s.%N)
    
    # Start server in background
    $cmd > /dev/null 2>&1 &
    local pid=$!
    
    # Wait for server to be ready
    while ! nc -z localhost $port 2>/dev/null; do
        sleep 0.01
    done
    
    local end_time=$(date +%s.%N)
    local startup_time=$(echo "$end_time - $start_time" | bc)
    
    echo "Startup time: ${startup_time}s"
    echo "$name,$startup_time" >> "$RESULTS_DIR/startup_times.csv"
    
    # Kill server
    kill $pid 2>/dev/null || true
    sleep 1
}

# Function to measure memory usage
measure_memory() {
    local pid=$1
    local name=$2
    
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        local mem=$(ps -o rss= -p $pid | awk '{print $1/1024}')
    else
        # Linux
        local mem=$(ps -o rss= -p $pid | awk '{print $1/1024}')
    fi
    
    echo "$name memory usage: ${mem}MB"
    echo "$name,$mem" >> "$RESULTS_DIR/memory_usage.csv"
}

# Build servers
echo -e "\n${BLUE}Building servers...${NC}"

cd ../go
go build -o ../benchmarks/hyperserve-go ./cmd/example-server
go build -o ../benchmarks/hyperserve-go-mcp ./examples/mcp

cd ../rust
cargo build --release --example basic
cargo build --release --example mcp_server
cp target/release/examples/basic ../benchmarks/hyperserve-rust
cp target/release/examples/mcp_server ../benchmarks/hyperserve-rust-mcp

cd ../benchmarks

# 1. Startup time comparison
echo -e "\n${BLUE}1. Startup Time Benchmark${NC}"
echo "name,startup_time_seconds" > "$RESULTS_DIR/startup_times.csv"

measure_startup_time "./hyperserve-go -port $GO_PORT" "Go" $GO_PORT
measure_startup_time "./hyperserve-rust" "Rust" $RUST_PORT

# 2. Memory usage under load
echo -e "\n${BLUE}2. Memory Usage Benchmark${NC}"
echo "name,memory_mb" > "$RESULTS_DIR/memory_usage.csv"

# Start servers
./hyperserve-go -port $GO_PORT > /dev/null 2>&1 &
GO_PID=$!
./hyperserve-rust > /dev/null 2>&1 &
RUST_PID=$!

sleep 2

# Measure initial memory
measure_memory $GO_PID "Go_initial"
measure_memory $RUST_PID "Rust_initial"

# Generate load
echo "Generating load..."
if command -v wrk &> /dev/null; then
    wrk -t4 -c100 -d10s "http://localhost:$GO_PORT/" > /dev/null 2>&1 &
    wrk -t4 -c100 -d10s "http://localhost:$RUST_PORT/" > /dev/null 2>&1 &
    wait
else
    for i in {1..1000}; do
        curl -s "http://localhost:$GO_PORT/" > /dev/null &
        curl -s "http://localhost:$RUST_PORT/" > /dev/null &
    done
    wait
fi

# Measure memory after load
measure_memory $GO_PID "Go_after_load"
measure_memory $RUST_PID "Rust_after_load"

kill $GO_PID $RUST_PID 2>/dev/null || true

# 3. Latency distribution
echo -e "\n${BLUE}3. Latency Distribution Benchmark${NC}"

# Start servers again
./hyperserve-go -port $GO_PORT > /dev/null 2>&1 &
GO_PID=$!
./hyperserve-rust > /dev/null 2>&1 &
RUST_PID=$!

sleep 2

if command -v wrk &> /dev/null; then
    echo "Measuring Go latency distribution..."
    wrk -t2 -c10 -d30s --latency "http://localhost:$GO_PORT/" > "$RESULTS_DIR/go_latency.txt"
    
    echo "Measuring Rust latency distribution..."
    wrk -t2 -c10 -d30s --latency "http://localhost:$RUST_PORT/" > "$RESULTS_DIR/rust_latency.txt"
fi

kill $GO_PID $RUST_PID 2>/dev/null || true

# 4. MCP Protocol Performance
echo -e "\n${BLUE}4. MCP Protocol Benchmark${NC}"

# Start MCP servers
HYPERSERVE_MCP=true ./hyperserve-go-mcp > /dev/null 2>&1 &
MCP_GO_PID=$!
./hyperserve-rust-mcp > /dev/null 2>&1 &
MCP_RUST_PID=$!

sleep 2

# Test MCP operations
echo "Testing MCP tool listing..."
for i in {1..100}; do
    curl -s -X POST "http://localhost:8082/mcp" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"tools/list","id":1}' > /dev/null &
done
wait
echo "Go MCP completed"

for i in {1..100}; do
    curl -s -X POST "http://localhost:8083/mcp" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"tools/list","id":1}' > /dev/null &
done
wait
echo "Rust MCP completed"

kill $MCP_GO_PID $MCP_RUST_PID 2>/dev/null || true

# 5. Generate final report
echo -e "\n${BLUE}Generating Report...${NC}"

cat > "$RESULTS_DIR/report.md" << EOF
# HyperServe Detailed Benchmark Report

Generated: $(date)

## 1. Startup Time

\`\`\`csv
$(cat "$RESULTS_DIR/startup_times.csv")
\`\`\`

## 2. Memory Usage

\`\`\`csv
$(cat "$RESULTS_DIR/memory_usage.csv")
\`\`\`

## 3. Latency Analysis

### Go Implementation
\`\`\`
$(grep -A 10 "Latency Distribution" "$RESULTS_DIR/go_latency.txt" 2>/dev/null || echo "No latency data")
\`\`\`

### Rust Implementation
\`\`\`
$(grep -A 10 "Latency Distribution" "$RESULTS_DIR/rust_latency.txt" 2>/dev/null || echo "No latency data")
\`\`\`

## Summary

- **Startup Time**: Compare startup performance between implementations
- **Memory Usage**: Analyze memory footprint under load
- **Latency**: Review response time distributions
- **MCP Performance**: Protocol-specific performance metrics

EOF

echo -e "\n${GREEN}Detailed benchmarks complete!${NC}"
echo "Results saved to: $RESULTS_DIR"
echo "Report: $RESULTS_DIR/report.md"

# Cleanup
rm -f hyperserve-go hyperserve-rust hyperserve-go-mcp hyperserve-rust-mcp