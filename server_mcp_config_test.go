package hyperserve

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestMCPProgrammaticConfigurationNoDoubleWarning tests that when MCP is configured
// programmatically with MCPDev(), we don't get duplicate configuration messages.
// This reproduces the issue seen in HF_DAW where "MCP auto-configured" appeared
// even though MCP was already configured via WithMCPSupport.
func TestMCPProgrammaticConfigurationNoDoubleWarning(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	oldLogger := logger
	logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defer func() { logger = oldLogger }()

	// Simulate HF_DAW's configuration approach
	serverOpts := []ServerOptionFunc{
		WithRateLimit(100, 200),
		WithCSPWebWorkerSupport(),
	}

	// Configure MCP with dev mode (like HF_DAW does)
	mcpConfigs := []MCPTransportConfig{MCPDev()}
	serverOpts = append(serverOpts, WithMCPSupport("TestApp", "1.0.0", mcpConfigs...))

	// Create server
	srv, err := NewServer(serverOpts...)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Check log output
	logOutput := buf.String()
	
	// Should see the developer mode warning
	if !strings.Contains(logOutput, "MCP DEVELOPER MODE ENABLED") {
		t.Error("Expected to see MCP developer mode warning")
	}

	// Should NOT see "MCP auto-configured from options" with dev=false
	if strings.Contains(logOutput, "MCP auto-configured from options") && 
	   strings.Contains(logOutput, "dev=false") {
		t.Error("Should not see auto-configuration message with dev=false when programmatically configured with MCPDev()")
	}

	// Should see that MCP was configured programmatically
	if !strings.Contains(logOutput, "MCP already configured programmatically") &&
	   !strings.Contains(logOutput, "MCP (Model Context Protocol) support enabled") {
		t.Error("Expected to see that MCP was configured")
	}

	// Verify MCP is actually enabled with dev mode
	if !srv.MCPEnabled() {
		t.Error("MCP should be enabled")
	}

	// Verify developer mode was applied
	if !srv.Options.mcpTransportOpts.developerMode {
		t.Error("Developer mode should be enabled in transport options")
	}
}

// TestMCPEnvironmentConfiguration tests that MCP can be configured via environment variables
func TestMCPEnvironmentConfiguration(t *testing.T) {
	// Set environment variables
	t.Setenv("HS_MCP_ENABLED", "true")
	t.Setenv("HS_MCP_SERVER_NAME", "EnvTestApp")
	t.Setenv("HS_MCP_SERVER_VERSION", "2.0.0")
	t.Setenv("HS_MCP_DEV", "true")

	// Capture log output
	var buf bytes.Buffer
	oldLogger := logger
	logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defer func() { logger = oldLogger }()

	// Create server without programmatic MCP configuration
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Check log output
	logOutput := buf.String()

	// Should see auto-configuration from environment
	if !strings.Contains(logOutput, "MCP auto-configured from environment/flags") {
		t.Error("Expected to see MCP auto-configured from environment")
	}

	// Should show dev=true
	if strings.Contains(logOutput, "dev=false") {
		t.Error("Should show dev=true when HS_MCP_DEV is set")
	}

	// Verify MCP is enabled
	if !srv.MCPEnabled() {
		t.Error("MCP should be enabled from environment")
	}
}

// TestMCPMixedConfiguration tests that programmatic configuration takes precedence
func TestMCPMixedConfiguration(t *testing.T) {
	// Set environment variables that would enable dev mode
	t.Setenv("HS_MCP_ENABLED", "true")
	t.Setenv("HS_MCP_SERVER_NAME", "MixedTestApp")
	t.Setenv("HS_MCP_DEV", "true")

	// Capture log output
	var buf bytes.Buffer
	oldLogger := logger
	logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defer func() { logger = oldLogger }()

	// But configure programmatically with observability mode instead
	srv, err := NewServer(
		WithMCPSupport("ProgrammaticApp", "3.0.0", MCPObservability()),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Check log output
	logOutput := buf.String()

	// Should NOT see auto-configuration (programmatic takes precedence)
	if strings.Contains(logOutput, "MCP auto-configured from environment") {
		t.Error("Should not auto-configure when programmatically configured")
	}

	// Should use programmatic name, not environment name
	if srv.Options.MCPServerName != "ProgrammaticApp" {
		t.Errorf("Expected programmatic name 'ProgrammaticApp', got %s", srv.Options.MCPServerName)
	}

	// Should have observability mode, not dev mode
	if srv.Options.mcpTransportOpts.developerMode {
		t.Error("Should not have developer mode when configured with observability")
	}
	if !srv.Options.mcpTransportOpts.observabilityMode {
		t.Error("Should have observability mode")
	}
}