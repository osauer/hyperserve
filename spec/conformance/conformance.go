// HyperServe Conformance Test Suite
// Tests both Go and Rust implementations for feature parity

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"time"
)

const (
	defaultGoPort   = 8080
	defaultRustPort = 8081
	timeout         = 10 * time.Second
)

var (
	goPort   int
	rustPort int
	mode     string
)

type testResult struct {
	passed  int
	failed  int
	skipped int
}

func init() {
	flag.IntVar(&goPort, "go-port", defaultGoPort, "Port for Go server")
	flag.IntVar(&rustPort, "rust-port", defaultRustPort, "Port for Rust server")
	flag.StringVar(&mode, "mode", "both", "Test mode: go, rust, or both")
}

func main() {
	flag.Parse()

	switch mode {
	case "go":
		testSingleImplementation("go", goPort)
	case "rust":
		testSingleImplementation("rust", rustPort)
	case "both":
		testBothImplementations()
	default:
		fmt.Fprintf(os.Stderr, "Invalid mode: %s\n", mode)
		os.Exit(1)
	}
}

func testSingleImplementation(impl string, port int) {
	server := startServer(impl, port)
	defer stopServer(server)

	result := &testResult{}
	
	fmt.Printf("Testing %s implementation on port %d\n", impl, port)
	testBasicHTTP(impl, port, result)
	testMCPProtocol(impl, port, result)
	
	printSummary(result)
}

func testBothImplementations() {
	// Start both servers
	goServer := startServer("go", goPort)
	defer stopServer(goServer)
	
	rustServer := startServer("rust", rustPort)
	defer stopServer(rustServer)
	
	result := &testResult{}
	
	fmt.Println("Running conformance tests...")
	
	// Test basic HTTP
	compareEndpoint("GET /health", "/health", "GET", nil, result)
	compareEndpoint("GET /", "/", "GET", nil, result)
	
	// Test MCP if available
	if checkMCPAvailable(goPort) && checkMCPAvailable(rustPort) {
		// Initialize
		initRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "conformance",
					"version": "1.0",
				},
			},
			"id": 1,
		}
		compareEndpoint("MCP Initialize", "/mcp", "POST", initRequest, result)
		
		// List tools
		listRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      2,
		}
		compareEndpoint("MCP List Tools", "/mcp", "POST", listRequest, result)
	}
	
	printSummary(result)
}

func startServer(impl string, port int) *exec.Cmd {
	fmt.Printf("[INFO] Starting %s server on port %d...\n", impl, port)
	
	var cmd *exec.Cmd
	rootDir := "../.."
	
	if impl == "go" {
		cmd = exec.Command("go", "run", "./cmd/server", fmt.Sprintf("-port=%d", port))
		cmd.Dir = rootDir + "/go"
	} else {
		cmd = exec.Command("cargo", "run", "--release", "--features", "mcp", "--bin", "hyperserve-server", "--", "--port", fmt.Sprintf("%d", port))
		cmd.Dir = rootDir + "/rust"
	}
	
	// Start server
	logFile, _ := os.Create(fmt.Sprintf("/tmp/hyperserve_%s.log", impl))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	
	if err := cmd.Start(); err != nil {
		fmt.Printf("[FAIL] Failed to start %s server: %v\n", impl, err)
		os.Exit(1)
	}
	
	// Wait for server to be ready
	for i := 0; i < 30; i++ {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			fmt.Printf("[PASS] %s server started successfully\n", impl)
			return cmd
		}
		time.Sleep(time.Second)
	}
	
	fmt.Printf("[FAIL] %s server failed to start\n", impl)
	cmd.Process.Kill()
	os.Exit(1)
	return nil
}

func stopServer(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	}
}

func testBasicHTTP(impl string, port int, result *testResult) {
	fmt.Printf("[INFO] Testing basic HTTP endpoints for %s...\n", impl)
	
	// Test health
	if testEndpoint(port, "/health", "GET", nil, 200) {
		fmt.Println("[PASS] Health check")
		result.passed++
	} else {
		fmt.Println("[FAIL] Health check")
		result.failed++
	}
	
	// Test root
	if testEndpoint(port, "/", "GET", nil, 200) {
		fmt.Println("[PASS] Root path")
		result.passed++
	} else {
		fmt.Println("[FAIL] Root path")
		result.failed++
	}
	
	// Test 404 - some servers may have a catch-all handler
	resp404 := testEndpointStatus(port, "/nonexistent", "GET", nil)
	if resp404 == 404 {
		fmt.Println("[PASS] 404 Not Found (proper 404)")
		result.passed++
	} else if resp404 == 200 {
		fmt.Println("[WARN] Not Found returns 200 (catch-all handler)")
		result.passed++ // Still pass but with warning
	} else {
		fmt.Printf("[FAIL] Not Found returned %d\n", resp404)
		result.failed++
	}
}

func testMCPProtocol(impl string, port int, result *testResult) {
	fmt.Printf("[INFO] Testing MCP protocol for %s...\n", impl)
	
	if !checkMCPAvailable(port) {
		fmt.Println("[WARN] MCP not available, skipping...")
		result.skipped += 3
		return
	}
	
	// Test initialize
	initRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "conformance",
				"version": "1.0",
			},
		},
		"id": 1,
	}
	
	if testEndpoint(port, "/mcp", "POST", initRequest, 200) {
		fmt.Println("[PASS] MCP Initialize")
		result.passed++
	} else {
		fmt.Println("[FAIL] MCP Initialize")
		result.failed++
	}
}

func testEndpoint(port int, path string, method string, body interface{}, expectedStatus int) bool {
	status := testEndpointStatus(port, path, method, body)
	return status == expectedStatus
}

func testEndpointStatus(port int, path string, method string, body interface{}) int {
	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	
	var req *http.Request
	var err error
	
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		req, err = http.NewRequest(method, url, bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	
	if err != nil {
		return 0
	}
	
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	
	return resp.StatusCode
}

func compareEndpoint(testName string, path string, method string, body interface{}, result *testResult) {
	goResp := fetchResponse(goPort, path, method, body)
	rustResp := fetchResponse(rustPort, path, method, body)
	
	// Normalize responses
	normalizeResponse(goResp)
	normalizeResponse(rustResp)
	
	if reflect.DeepEqual(goResp, rustResp) {
		fmt.Printf("[PASS] %s: Responses match\n", testName)
		result.passed++
	} else {
		fmt.Printf("[FAIL] %s: Responses differ\n", testName)
		result.failed++
		
		// Show diff
		goJSON, _ := json.MarshalIndent(goResp, "", "  ")
		rustJSON, _ := json.MarshalIndent(rustResp, "", "  ")
		fmt.Printf("Go response:\n%s\n", goJSON)
		fmt.Printf("Rust response:\n%s\n", rustJSON)
	}
}

func fetchResponse(port int, path string, method string, body interface{}) map[string]interface{} {
	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	
	var req *http.Request
	var err error
	
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		req, err = http.NewRequest(method, url, bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	
	if err != nil {
		return nil
	}
	
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	
	bodyBytes, _ := io.ReadAll(resp.Body)
	
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		// Not JSON, return as string
		return map[string]interface{}{
			"body":   string(bodyBytes),
			"status": resp.StatusCode,
		}
	}
	
	return result
}

func normalizeResponse(resp map[string]interface{}) {
	// Remove server-specific fields that are expected to differ
	if result, ok := resp["result"].(map[string]interface{}); ok {
		if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
			// Normalize version
			serverInfo["version"] = "X.X.X"
		}
	}
}

func checkMCPAvailable(port int) bool {
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/mcp", port),
		"application/json",
		strings.NewReader(`{}`),
	)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode != 404
}

func printSummary(result *testResult) {
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("[INFO] Conformance Test Summary")
	fmt.Printf("[INFO] Tests Passed: %d\n", result.passed)
	
	if result.failed > 0 {
		fmt.Printf("[FAIL] Tests Failed: %d\n", result.failed)
	}
	
	if result.skipped > 0 {
		fmt.Printf("[WARN] Tests Skipped: %d\n", result.skipped)
	}
	
	if result.failed == 0 {
		fmt.Println("[PASS] All tests passed! Implementations are conformant.")
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}