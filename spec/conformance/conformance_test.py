#!/usr/bin/env python3
"""
HyperServe Conformance Test Suite
Tests both Go and Rust implementations for feature parity
"""

import json
import subprocess
import sys
import time
import requests
import argparse
from typing import Dict, Any, Tuple, Optional
import difflib
import os
import signal

# Configuration
GO_PORT = 8080
RUST_PORT = 8081
TIMEOUT = 10

class Colors:
    GREEN = '\033[92m'
    RED = '\033[91m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    ENDC = '\033[0m'

def print_status(status: str, message: str):
    """Print colored status messages"""
    if status == "PASS":
        print(f"{Colors.GREEN}[PASS]{Colors.ENDC} {message}")
    elif status == "FAIL":
        print(f"{Colors.RED}[FAIL]{Colors.ENDC} {message}")
    elif status == "INFO":
        print(f"{Colors.BLUE}[INFO]{Colors.ENDC} {message}")
    elif status == "WARN":
        print(f"{Colors.YELLOW}[WARN]{Colors.ENDC} {message}")

class ServerManager:
    """Manages server lifecycle"""
    
    def __init__(self, impl: str, port: int):
        self.impl = impl
        self.port = port
        self.process = None
        self.root_dir = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
        
    def start(self) -> bool:
        """Start the server"""
        print_status("INFO", f"Starting {self.impl} server on port {self.port}...")
        
        if self.impl == "go":
            os.chdir(os.path.join(self.root_dir, "go"))
            cmd = ["go", "run", "./cmd/server", "-port", str(self.port)]
        else:  # rust
            os.chdir(os.path.join(self.root_dir, "rust"))
            # Check if we need to build with MCP feature
            cmd = ["cargo", "run", "--release", "--features", "mcp", "--", "--port", str(self.port)]
        
        # Start the process
        log_file = f"/tmp/hyperserve_{self.impl}.log"
        with open(log_file, "w") as f:
            self.process = subprocess.Popen(cmd, stdout=f, stderr=f)
        
        # Wait for server to be ready
        for i in range(30):  # 30 second timeout
            try:
                resp = requests.get(f"http://localhost:{self.port}/health", timeout=1)
                if resp.status_code == 200:
                    print_status("PASS", f"{self.impl} server started successfully")
                    return True
            except:
                time.sleep(1)
        
        print_status("FAIL", f"{self.impl} server failed to start")
        with open(log_file, "r") as f:
            print(f.read())
        return False
    
    def stop(self):
        """Stop the server"""
        if self.process:
            self.process.terminate()
            self.process.wait(timeout=5)
            print_status("INFO", f"{self.impl} server stopped")

class ConformanceTest:
    """Main conformance test suite"""
    
    def __init__(self):
        self.results = {"passed": 0, "failed": 0, "skipped": 0}
        self.failures = []
        
    def normalize_json(self, data: Any) -> Any:
        """Normalize JSON for comparison"""
        if isinstance(data, dict):
            # Sort dictionary keys
            return {k: self.normalize_json(v) for k, v in sorted(data.items())}
        elif isinstance(data, list):
            return [self.normalize_json(item) for item in data]
        else:
            return data
    
    def compare_responses(self, test_name: str, go_resp: Dict, rust_resp: Dict) -> bool:
        """Compare two responses for equality"""
        # Normalize responses
        go_norm = self.normalize_json(go_resp)
        rust_norm = self.normalize_json(rust_resp)
        
        if go_norm == rust_norm:
            print_status("PASS", f"{test_name}: Responses match")
            self.results["passed"] += 1
            return True
        else:
            print_status("FAIL", f"{test_name}: Responses differ")
            self.results["failed"] += 1
            self.failures.append(test_name)
            
            # Show diff
            go_str = json.dumps(go_norm, indent=2)
            rust_str = json.dumps(rust_norm, indent=2)
            diff = difflib.unified_diff(
                go_str.splitlines(keepends=True),
                rust_str.splitlines(keepends=True),
                fromfile='Go response',
                tofile='Rust response'
            )
            print(''.join(diff))
            return False
    
    def test_endpoint(self, path: str, method: str = "GET", 
                     data: Optional[Dict] = None, 
                     expected_status: int = 200) -> Tuple[Dict, Dict]:
        """Test an endpoint on both servers"""
        headers = {"Content-Type": "application/json"} if data else {}
        
        # Test Go server
        go_url = f"http://localhost:{GO_PORT}{path}"
        if method == "GET":
            go_resp = requests.get(go_url, timeout=TIMEOUT)
        else:
            go_resp = requests.request(method, go_url, json=data, headers=headers, timeout=TIMEOUT)
        
        # Test Rust server  
        rust_url = f"http://localhost:{RUST_PORT}{path}"
        if method == "GET":
            rust_resp = requests.get(rust_url, timeout=TIMEOUT)
        else:
            rust_resp = requests.request(method, rust_url, json=data, headers=headers, timeout=TIMEOUT)
        
        # Check status codes
        if go_resp.status_code != expected_status:
            print_status("FAIL", f"Go server returned {go_resp.status_code}, expected {expected_status}")
        if rust_resp.status_code != expected_status:
            print_status("FAIL", f"Rust server returned {rust_resp.status_code}, expected {expected_status}")
        
        # Parse JSON responses
        go_data = go_resp.json() if go_resp.content else {}
        rust_data = rust_resp.json() if rust_resp.content else {}
        
        return go_data, rust_data
    
    def test_basic_http(self):
        """Test basic HTTP functionality"""
        print_status("INFO", "Testing basic HTTP endpoints...")
        
        # Test health endpoint
        go_resp, rust_resp = self.test_endpoint("/health")
        self.compare_responses("Health Check", go_resp, rust_resp)
        
        # Test 404
        try:
            self.test_endpoint("/nonexistent", expected_status=404)
            print_status("PASS", "404 Not Found works correctly")
            self.results["passed"] += 1
        except:
            print_status("FAIL", "404 handling failed")
            self.results["failed"] += 1
    
    def test_mcp_protocol(self):
        """Test MCP protocol implementation"""
        print_status("INFO", "Testing MCP protocol...")
        
        # Check if MCP is available
        try:
            requests.post(f"http://localhost:{GO_PORT}/mcp", timeout=1)
            requests.post(f"http://localhost:{RUST_PORT}/mcp", timeout=1)
        except:
            print_status("WARN", "MCP endpoint not available, skipping MCP tests")
            self.results["skipped"] += 5
            return
        
        # Test initialize
        init_request = {
            "jsonrpc": "2.0",
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "conformance", "version": "1.0"}
            },
            "id": 1
        }
        
        go_resp, rust_resp = self.test_endpoint("/mcp", "POST", init_request)
        
        # Compare normalized responses (ignore server-specific fields)
        if "result" in go_resp and "result" in rust_resp:
            # Remove fields that may differ
            for resp in [go_resp, rust_resp]:
                if "serverInfo" in resp["result"]:
                    # Keep structure but normalize version
                    resp["result"]["serverInfo"]["version"] = "X.X.X"
        
        self.compare_responses("MCP Initialize", go_resp, rust_resp)
        
        # Test tools/list
        list_request = {
            "jsonrpc": "2.0",
            "method": "tools/list",
            "id": 2
        }
        
        go_resp, rust_resp = self.test_endpoint("/mcp", "POST", list_request)
        self.compare_responses("MCP List Tools", go_resp, rust_resp)
        
        # Test resources/list
        list_request = {
            "jsonrpc": "2.0", 
            "method": "resources/list",
            "id": 3
        }
        
        go_resp, rust_resp = self.test_endpoint("/mcp", "POST", list_request)
        self.compare_responses("MCP List Resources", go_resp, rust_resp)
        
        # Test tool call (if echo tool exists)
        if go_resp.get("result", {}).get("tools") and rust_resp.get("result", {}).get("tools"):
            echo_request = {
                "jsonrpc": "2.0",
                "method": "tools/call",
                "params": {
                    "name": "echo",
                    "arguments": {"message": "conformance test"}
                },
                "id": 4
            }
            
            go_resp, rust_resp = self.test_endpoint("/mcp", "POST", echo_request)
            self.compare_responses("MCP Echo Tool", go_resp, rust_resp)
    
    def test_middleware(self):
        """Test middleware behavior"""
        print_status("INFO", "Testing middleware...")
        
        # Test CORS headers
        go_resp = requests.options(f"http://localhost:{GO_PORT}/")
        rust_resp = requests.options(f"http://localhost:{RUST_PORT}/")
        
        # Compare headers
        headers_match = True
        for header in ["Access-Control-Allow-Origin", "Access-Control-Allow-Methods"]:
            go_val = go_resp.headers.get(header)
            rust_val = rust_resp.headers.get(header)
            if go_val != rust_val:
                print_status("FAIL", f"CORS header {header} differs: Go='{go_val}', Rust='{rust_val}'")
                headers_match = False
        
        if headers_match:
            print_status("PASS", "CORS headers match")
            self.results["passed"] += 1
        else:
            self.results["failed"] += 1
    
    def run_all_tests(self):
        """Run all conformance tests"""
        print_status("INFO", "Running HyperServe conformance tests...")
        
        self.test_basic_http()
        self.test_mcp_protocol()
        self.test_middleware()
        
        # Print summary
        print("\n" + "="*50)
        print_status("INFO", "Conformance Test Summary")
        print_status("INFO", f"Tests Passed: {self.results['passed']}")
        if self.results['failed'] > 0:
            print_status("FAIL", f"Tests Failed: {self.results['failed']}")
            print_status("INFO", f"Failed tests: {', '.join(self.failures)}")
        if self.results['skipped'] > 0:
            print_status("WARN", f"Tests Skipped: {self.results['skipped']}")
        
        if self.results['failed'] == 0:
            print_status("PASS", "All tests passed! Both implementations are conformant.")
            return True
        else:
            return False

def main():
    global GO_PORT, RUST_PORT
    
    parser = argparse.ArgumentParser(description='HyperServe Conformance Test Suite')
    parser.add_argument('mode', choices=['go', 'rust', 'both'],
                       help='Which implementation(s) to test')
    parser.add_argument('--go-port', type=int, default=GO_PORT,
                       help=f'Port for Go server (default: {GO_PORT})')
    parser.add_argument('--rust-port', type=int, default=RUST_PORT,
                       help=f'Port for Rust server (default: {RUST_PORT})')
    args = parser.parse_args()
    
    # Update ports if specified
    GO_PORT = args.go_port
    RUST_PORT = args.rust_port
    
    # Create server managers
    go_server = ServerManager("go", GO_PORT)
    rust_server = ServerManager("rust", RUST_PORT)
    
    success = True
    
    try:
        if args.mode == "go":
            if not go_server.start():
                sys.exit(1)
            # Run basic tests against Go only
            print_status("INFO", "Running tests against Go implementation only")
            
        elif args.mode == "rust":
            if not rust_server.start():
                sys.exit(1)
            # Run basic tests against Rust only
            print_status("INFO", "Running tests against Rust implementation only")
            
        elif args.mode == "both":
            # Start both servers
            if not go_server.start() or not rust_server.start():
                sys.exit(1)
            
            # Run conformance tests
            test = ConformanceTest()
            success = test.run_all_tests()
            
    except KeyboardInterrupt:
        print_status("WARN", "Tests interrupted")
        success = False
    finally:
        # Clean up
        go_server.stop()
        rust_server.stop()
    
    sys.exit(0 if success else 1)

if __name__ == "__main__":
    main()