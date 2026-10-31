package main

import (
	"testing"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/mcp"
)

func TestMCPDevOpsPreset(t *testing.T) {
	// Example 1: Basic MCP support (no built-in tools/resources)
	app1, _ := hyperserve.New(
		hyperserve.WithMCPSupport("MyApp", "1.0.0"),
	)
	// MCP is enabled but no built-in tools or resources
	_ = app1

	// Example 2: MCP with full built-in tools and resources
	app2, _ := hyperserve.New(
		hyperserve.WithMCPSupport("MyApp", "1.0.0"),
		hyperserve.WithMCPBuiltinTools(true),
		hyperserve.WithMCPBuiltinResources(true),
	)
	// All built-in tools and resources are available
	_ = app2

	// Example 3: MCP with Observability (minimal, secure monitoring)
	app3, _ := hyperserve.New(
		hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPObservability()),
	)
	// Only 3 essential observability resources, no tools, no sensitive data exposed
	_ = app3

	// Example 4: MCP Observability with STDIO transport for Claude Desktop
	app4, _ := hyperserve.New(
		hyperserve.WithMCPSupport("MyApp", "1.0.0",
			mcp.OverStdio(),
			hyperserve.MCPObservability(),
		),
	)
	// Observability resources available via STDIO for Claude Desktop integration
	_ = app4
}
