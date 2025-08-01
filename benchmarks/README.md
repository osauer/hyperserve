# HyperServe Benchmarks

Comparative performance benchmarks between Go and Rust implementations.

## Quick Start

```bash
# Run all benchmarks
./run_benchmarks.sh

# Run specific benchmark
./run_benchmarks.sh http_basic
```

## Benchmarks

### 1. HTTP Basic Performance
- Simple GET requests
- POST with small/medium/large payloads
- Concurrent connections

### 2. Routing Performance
- Static routes
- Parameterized routes
- Deep nested routes

### 3. Middleware Overhead
- No middleware baseline
- Single middleware
- Multiple middleware chain

### 4. WebSocket Performance
- Connection establishment
- Message throughput
- Concurrent connections

### 5. MCP Protocol Performance
- Tool listing
- Resource access
- Concurrent RPC calls

## Results

Results are saved to `results/` directory with timestamps.

## Requirements

- Go 1.24+
- Rust 1.70+
- wrk or ab (Apache Bench)
- Python 3 (for result analysis)