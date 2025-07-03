#!/bin/bash

# Test script for MCP stdio server

echo "Testing MCP stdio server..."

# Test 1: Initialize
echo "1. Initialize:"
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | go run main.go 2>/dev/null | jq .

# Test 2: List tools
echo -e "\n2. List tools:"
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | go run main.go 2>/dev/null | jq .

# Test 3: Calculator
echo -e "\n3. Calculator (15 * 4):"
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculator","arguments":{"operation":"multiply","a":15,"b":4}}}' | go run main.go 2>/dev/null | jq .

# Test 4: List directory
echo -e "\n4. List sandbox directory:"
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_directory","arguments":{"path":"."}}}' | go run main.go 2>/dev/null | jq .

# Test 5: Read file
echo -e "\n5. Read hello.txt:"
echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"hello.txt"}}}' | go run main.go 2>/dev/null | jq .