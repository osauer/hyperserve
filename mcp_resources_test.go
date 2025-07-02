package hyperserve

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSystemResource(t *testing.T) {
	resource := NewSystemResource()
	
	// Test resource metadata
	if resource.URI() != "system://runtime/info" {
		t.Errorf("Expected URI 'system://runtime/info', got %s", resource.URI())
	}
	
	if resource.Name() != "System Information" {
		t.Errorf("Expected name 'System Information', got %s", resource.Name())
	}
	
	if resource.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	if resource.MimeType() != "application/json" {
		t.Errorf("Expected mime type 'application/json', got %s", resource.MimeType())
	}
	
	// Test reading system information
	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Failed to read system resource: %v", err)
	}
	
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	
	// Verify it's valid JSON
	var systemInfo map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &systemInfo); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}
	
	// Check for expected fields
	if _, exists := systemInfo["go"]; !exists {
		t.Error("Expected 'go' field in system info")
	}
	
	if _, exists := systemInfo["memory"]; !exists {
		t.Error("Expected 'memory' field in system info")
	}
	
	if _, exists := systemInfo["timestamp"]; !exists {
		t.Error("Expected 'timestamp' field in system info")
	}
	
	// Test list method
	uris, err := resource.List()
	if err != nil {
		t.Fatalf("Failed to list system resource: %v", err)
	}
	
	if len(uris) != 1 {
		t.Errorf("Expected 1 URI, got %d", len(uris))
	}
	
	if uris[0] != resource.URI() {
		t.Errorf("Expected URI %s, got %s", resource.URI(), uris[0])
	}
}

func TestConfigResource(t *testing.T) {
	// Create test server options
	options := &ServerOptions{
		Addr:            ":8080",
		EnableTLS:       false,
		HealthAddr:      ":9080",
		RateLimit:       100,
		Burst:           200,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     120 * time.Second,
		StaticDir:       "static/",
		TemplateDir:     "templates/",
		RunHealthServer: true,
		ChaosMode:       false,
		FIPSMode:        false,
		HardenedMode:    false,
	}
	
	resource := NewConfigResource(options)
	
	// Test resource metadata
	if resource.URI() != "config://server/options" {
		t.Errorf("Expected URI 'config://server/options', got %s", resource.URI())
	}
	
	if resource.Name() != "Server Configuration" {
		t.Errorf("Expected name 'Server Configuration', got %s", resource.Name())
	}
	
	if resource.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	if resource.MimeType() != "application/json" {
		t.Errorf("Expected mime type 'application/json', got %s", resource.MimeType())
	}
	
	// Test reading configuration
	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Failed to read config resource: %v", err)
	}
	
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	
	// Verify it's valid JSON
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &config); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}
	
	// Check for expected fields
	if config["addr"] != ":8080" {
		t.Errorf("Expected addr ':8080', got %v", config["addr"])
	}
	
	if config["enableTLS"] != false {
		t.Errorf("Expected enableTLS false, got %v", config["enableTLS"])
	}
	
	if config["rateLimit"] != float64(100) {
		t.Errorf("Expected rateLimit 100, got %v", config["rateLimit"])
	}
	
	// Ensure sensitive data is not included (AuthTokenValidatorFunc should not be serialized)
	if _, exists := config["authTokenValidatorFunc"]; exists {
		t.Error("Sensitive data (authTokenValidatorFunc) should not be included")
	}
	
	// Test list method
	uris, err := resource.List()
	if err != nil {
		t.Fatalf("Failed to list config resource: %v", err)
	}
	
	if len(uris) != 1 {
		t.Errorf("Expected 1 URI, got %d", len(uris))
	}
	
	if uris[0] != resource.URI() {
		t.Errorf("Expected URI %s, got %s", resource.URI(), uris[0])
	}
}

func TestMetricsResource(t *testing.T) {
	// Create a test server
	srv := &Server{
		totalRequests:     &atomic.Uint64{},
		totalResponseTime: &atomic.Int64{},
		isRunning:         &atomic.Bool{},
		isReady:           &atomic.Bool{},
		serverStart:       time.Now().Add(-time.Hour), // Started 1 hour ago
	}
	
	// Set some test values
	srv.totalRequests.Store(100)
	srv.totalResponseTime.Store(50000) // 50ms total
	srv.isRunning.Store(true)
	srv.isReady.Store(true)
	
	resource := NewMetricsResource(srv)
	
	// Test resource metadata
	if resource.URI() != "metrics://server/stats" {
		t.Errorf("Expected URI 'metrics://server/stats', got %s", resource.URI())
	}
	
	if resource.Name() != "Server Metrics" {
		t.Errorf("Expected name 'Server Metrics', got %s", resource.Name())
	}
	
	if resource.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	if resource.MimeType() != "application/json" {
		t.Errorf("Expected mime type 'application/json', got %s", resource.MimeType())
	}
	
	// Test reading metrics
	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Failed to read metrics resource: %v", err)
	}
	
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	
	// Verify it's valid JSON
	var metrics map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &metrics); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}
	
	// Check for expected fields
	if _, exists := metrics["uptime"]; !exists {
		t.Error("Expected 'uptime' field in metrics")
	}
	
	if metrics["totalRequests"] != float64(100) {
		t.Errorf("Expected totalRequests 100, got %v", metrics["totalRequests"])
	}
	
	if metrics["isRunning"] != true {
		t.Errorf("Expected isRunning true, got %v", metrics["isRunning"])
	}
	
	if metrics["isReady"] != true {
		t.Errorf("Expected isReady true, got %v", metrics["isReady"])
	}
	
	// Test list method
	uris, err := resource.List()
	if err != nil {
		t.Fatalf("Failed to list metrics resource: %v", err)
	}
	
	if len(uris) != 1 {
		t.Errorf("Expected 1 URI, got %d", len(uris))
	}
	
	if uris[0] != resource.URI() {
		t.Errorf("Expected URI %s, got %s", resource.URI(), uris[0])
	}
}

func TestLogResource(t *testing.T) {
	resource := NewLogResource(5) // Max 5 entries
	
	// Test resource metadata
	if resource.URI() != "logs://server/recent" {
		t.Errorf("Expected URI 'logs://server/recent', got %s", resource.URI())
	}
	
	if resource.Name() != "Recent Log Entries" {
		t.Errorf("Expected name 'Recent Log Entries', got %s", resource.Name())
	}
	
	if resource.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	if resource.MimeType() != "text/plain" {
		t.Errorf("Expected mime type 'text/plain', got %s", resource.MimeType())
	}
	
	// Test reading empty log
	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Failed to read log resource: %v", err)
	}
	
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	
	if !strings.Contains(resultStr, "No log entries captured") {
		t.Error("Expected message about no log entries")
	}
	
	// Add some log entries
	resource.AddLogEntry("First log entry")
	resource.AddLogEntry("Second log entry")
	resource.AddLogEntry("Third log entry")
	
	// Test reading log with entries
	result, err = resource.Read()
	if err != nil {
		t.Fatalf("Failed to read log resource with entries: %v", err)
	}
	
	resultStr, ok = result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	
	if !strings.Contains(resultStr, "First log entry") {
		t.Error("Expected first log entry in result")
	}
	
	if !strings.Contains(resultStr, "Second log entry") {
		t.Error("Expected second log entry in result")
	}
	
	if !strings.Contains(resultStr, "Third log entry") {
		t.Error("Expected third log entry in result")
	}
	
	// Test log rotation (add more entries than max size)
	resource.AddLogEntry("Fourth log entry")
	resource.AddLogEntry("Fifth log entry")
	resource.AddLogEntry("Sixth log entry") // This should cause rotation
	
	result, err = resource.Read()
	if err != nil {
		t.Fatalf("Failed to read log resource after rotation: %v", err)
	}
	
	resultStr, ok = result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	
	// First entry should be rotated out
	if strings.Contains(resultStr, "First log entry") {
		t.Error("First log entry should have been rotated out")
	}
	
	// Sixth entry should be present
	if !strings.Contains(resultStr, "Sixth log entry") {
		t.Error("Expected sixth log entry in result")
	}
	
	// Test list method
	uris, err := resource.List()
	if err != nil {
		t.Fatalf("Failed to list log resource: %v", err)
	}
	
	if len(uris) != 1 {
		t.Errorf("Expected 1 URI, got %d", len(uris))
	}
	
	if uris[0] != resource.URI() {
		t.Errorf("Expected URI %s, got %s", resource.URI(), uris[0])
	}
}