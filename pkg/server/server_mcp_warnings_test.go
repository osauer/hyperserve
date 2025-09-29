package server

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// TestMCPDevModeWarnings verifies the expected warning behavior when MCP developer mode is enabled
func TestMCPDevModeWarnings(t *testing.T) {
	tests := []struct {
		name               string
		setup              func() (*Server, *bytes.Buffer)
		expectedHFWarnings int // Application's own warning
		expectedHSWarnings int // HyperServe's warning
		description        string
	}{
		{
			name: "HF_DAW style with application warning",
			setup: func() (*Server, *bytes.Buffer) {
				var buf bytes.Buffer

				// Capture hyperserve's slog output
				oldLogger := logger
				logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
				defer func() { logger = oldLogger }()

				// Also capture standard log output (what HF_DAW uses)
				oldLogOutput := log.Writer()
				log.SetOutput(&buf)
				defer log.SetOutput(oldLogOutput)

				// Simulate HF_DAW's approach
				mcpConfigs := []MCPTransportConfig{MCPDev()}
				log.Println("⚠️  MCP Developer Mode enabled - use for development only!") // HF_DAW's warning

				srv, _ := NewServer(
					WithMCPSupport("Test App", "1.0.0", mcpConfigs...),
				)

				return srv, &buf
			},
			expectedHFWarnings: 1,
			expectedHSWarnings: 1,
			description:        "Application adds its own warning + HyperServe adds its warning = 2 total",
		},
		{
			name: "Direct MCPDev without application warning",
			setup: func() (*Server, *bytes.Buffer) {
				var buf bytes.Buffer

				// Capture hyperserve's slog output
				oldLogger := logger
				logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
				defer func() { logger = oldLogger }()

				// No application warning here
				srv, _ := NewServer(
					WithMCPSupport("Test App", "1.0.0", MCPDev()),
				)

				return srv, &buf
			},
			expectedHFWarnings: 0,
			expectedHSWarnings: 1,
			description:        "Only HyperServe's warning appears = 1 total",
		},
		{
			name: "Environment variable configuration",
			setup: func() (*Server, *bytes.Buffer) {
				var buf bytes.Buffer

				// Set environment variables
				t.Setenv("HS_MCP_ENABLED", "true")
				t.Setenv("HS_MCP_SERVER_NAME", "EnvApp")
				t.Setenv("HS_MCP_SERVER_VERSION", "1.0.0")
				t.Setenv("HS_MCP_DEV", "true")

				// Capture hyperserve's slog output
				oldLogger := logger
				logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
				defer func() { logger = oldLogger }()

				srv, _ := NewServer() // Auto-configured from environment

				return srv, &buf
			},
			expectedHFWarnings: 0,
			expectedHSWarnings: 1,
			description:        "Environment configuration only shows HyperServe's warning = 1 total",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, buf := tt.setup()
			logOutput := buf.String()

			// Count warnings
			hfWarningCount := strings.Count(logOutput, "⚠️  MCP Developer Mode enabled - use for development only!")
			hsWarningCount := strings.Count(logOutput, "⚠️  MCP DEVELOPER MODE ENABLED ⚠️")

			// Verify expected counts
			if hfWarningCount != tt.expectedHFWarnings {
				t.Errorf("Expected %d HF_DAW-style warnings, got %d", tt.expectedHFWarnings, hfWarningCount)
			}

			if hsWarningCount != tt.expectedHSWarnings {
				t.Errorf("Expected %d HyperServe warnings, got %d", tt.expectedHSWarnings, hsWarningCount)
			}

			// Verify MCP is enabled
			if !srv.MCPEnabled() {
				t.Error("MCP should be enabled")
			}

			t.Logf("Test case: %s", tt.description)
			t.Logf("Total warnings: %d (HF: %d, HS: %d)", hfWarningCount+hsWarningCount, hfWarningCount, hsWarningCount)
		})
	}
}

// TestMCPWarningDocumentation documents the expected warning behavior
func TestMCPWarningDocumentation(t *testing.T) {
	// This test serves as documentation of the expected behavior
	t.Log("MCP Developer Mode Warning Behavior:")
	t.Log("1. HyperServe always logs ONE warning when MCPDev() is used")
	t.Log("2. Applications may add their own warnings (like HF_DAW does)")
	t.Log("3. Two warnings is normal when the application adds its own warning")
	t.Log("4. This is NOT a bug - both components are being responsible about security")
	t.Log("")
	t.Log("Example from HF_DAW:")
	t.Log("  - HF_DAW logs: '⚠️  MCP Developer Mode enabled - use for development only!'")
	t.Log("  - HyperServe logs: '⚠️  MCP DEVELOPER MODE ENABLED ⚠️'")
	t.Log("  - Total: 2 warnings (expected behavior)")
}
