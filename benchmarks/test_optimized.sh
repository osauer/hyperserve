#!/bin/bash

echo "Testing Optimized Thread Pool"
echo "============================"
echo "WARNING: This uses experimental features that may be unstable"
echo ""

# Build the performance tuning example
cd /Users/osauer/dev/hyperserve/rust
cargo build --release --example performance_tuning 2>/dev/null

# Kill any existing server
pkill performance_tuning 2>/dev/null || true
sleep 1

# Start server
echo "Starting server with optimized thread pool..."
./target/release/examples/performance_tuning &
SERVER_PID=$!

sleep 2

# Test server is running
echo "Testing server..."
if curl -s http://127.0.0.1:8082/ > /dev/null; then
    echo "Server is running OK"
else
    echo "Server failed to start"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi

echo ""
echo "Quick performance test (100 connections):"
ab -n 1000 -c 100 -q http://127.0.0.1:8082/ 2>/dev/null | grep "Requests per second"

# Cleanup
kill $SERVER_PID 2>/dev/null || true

echo ""
echo "Note: The optimized pool is currently disabled for stability."
echo "To fully enable it, uncomment the OptimizedPool creation in lib.rs"