#!/bin/bash

echo "Simple Benchmark Test"
echo "===================="

# Kill existing servers
pkill hyperserve 2>/dev/null || true
pkill basic 2>/dev/null || true
sleep 1

# Build fresh
echo "Building servers..."
cd /Users/osauer/dev/hyperserve/go
go build -o ../benchmarks/go-server ./cmd/example-server/main.go

cd /Users/osauer/dev/hyperserve/rust  
cargo build --release --example basic 2>/dev/null
cp target/release/examples/basic ../benchmarks/rust-server

cd /Users/osauer/dev/hyperserve/benchmarks

# Start servers
echo "Starting servers..."
./go-server -port 8080 > /dev/null 2>&1 &
GO_PID=$!

./rust-server > /dev/null 2>&1 &
RUST_PID=$!

sleep 3

# Test servers
echo -e "\nTesting servers..."
curl -s http://127.0.0.1:8080/ > /dev/null && echo "Go server: OK" || echo "Go server: FAILED"
curl -s http://127.0.0.1:8081/ > /dev/null && echo "Rust server: OK" || echo "Rust server: FAILED"

echo -e "\n=== Performance Results ==="

# Single connection
echo -e "\n1. Single Connection (Latency Test)"
echo "Go:"
ab -n 1000 -c 1 -q http://127.0.0.1:8080/ 2>/dev/null | grep -E "(Requests per second|Time per request|Failed)" | head -3

echo -e "\nRust:"
ab -n 1000 -c 1 -q http://127.0.0.1:8081/ 2>/dev/null | grep -E "(Requests per second|Time per request|Failed)" | head -3

# Normal load
echo -e "\n2. Normal Load (100 connections)"
echo "Go:"
ab -n 10000 -c 100 -q http://127.0.0.1:8080/ 2>/dev/null | grep "Requests per second"

echo "Rust:"
ab -n 10000 -c 100 -q http://127.0.0.1:8081/ 2>/dev/null | grep "Requests per second"

# High load
echo -e "\n3. High Load (500 connections)"
echo "Go:"
ab -n 5000 -c 500 -q http://127.0.0.1:8080/ 2>/dev/null | grep "Requests per second"

echo "Rust:"
ab -n 5000 -c 500 -q http://127.0.0.1:8081/ 2>/dev/null | grep "Requests per second"

# Cleanup
kill $GO_PID $RUST_PID 2>/dev/null || true

echo -e "\nDone!"