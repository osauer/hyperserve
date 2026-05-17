package builtin

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve/pkg/server"
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
	var systemInfo map[string]any
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
	options := &server.ServerOptions{
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
		RunHealthServer: false,
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
	var config map[string]any
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
	srv, err := server.NewServer()
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	srv.SetMetrics(100, 5000)

	resource := NewMetricsResource(srv)

	if resource.URI() != "metrics://server/stats" {
		t.Errorf("URI: want metrics://server/stats, got %s", resource.URI())
	}
	if resource.Name() != "Server Metrics" {
		t.Errorf("Name: want 'Server Metrics', got %s", resource.Name())
	}
	if resource.MimeType() != "application/json" {
		t.Errorf("MimeType: want application/json, got %s", resource.MimeType())
	}

	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	raw, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	var metrics map[string]any
	if err := json.Unmarshal([]byte(raw), &metrics); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, field := range []string{"uptime", "totalRequests", "isRunning", "isReady"} {
		if _, ok := metrics[field]; !ok {
			t.Errorf("missing field %q in metrics payload", field)
		}
	}
	if got, want := metrics["totalRequests"], float64(100); got != want {
		t.Errorf("totalRequests: want %v, got %v", want, got)
	}

	uris, err := resource.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(uris) != 1 || uris[0] != resource.URI() {
		t.Errorf("List = %v, want [%s]", uris, resource.URI())
	}
}

func TestServerLogResource(t *testing.T) {
	resource := NewServerLogResource(3)

	if resource.URI() != "logs://server/recent" {
		t.Errorf("expected URI logs://server/recent, got %s", resource.URI())
	}
	if resource.MimeType() != "application/json" {
		t.Errorf("expected application/json, got %s", resource.MimeType())
	}

	// Ingest 4 entries; oldest should rotate out.
	ctx := context.Background()
	for i, msg := range []string{"first", "second", "third", "fourth"} {
		rec := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
		rec.AddAttrs(slog.Int("i", i))
		if err := resource.Handle(ctx, rec); err != nil {
			t.Fatalf("Handle(%s) failed: %v", msg, err)
		}
	}

	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	raw, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	var payload struct {
		Logs []struct {
			Message string `json:"msg"`
		} `json:"logs"`
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if payload.Count != 3 {
		t.Errorf("expected 3 entries after rotation, got %d", payload.Count)
	}
	if !payload.Truncated {
		t.Error("expected truncated=true at capacity")
	}
	got := []string{payload.Logs[0].Message, payload.Logs[1].Message, payload.Logs[2].Message}
	want := []string{"second", "third", "fourth"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: want %q got %q", i, want[i], got[i])
		}
	}
	if strings.Contains(raw, `"first"`) {
		t.Error("first entry should have been rotated out")
	}

	uris, err := resource.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(uris) != 1 || uris[0] != resource.URI() {
		t.Errorf("List = %v, want [%s]", uris, resource.URI())
	}
}
