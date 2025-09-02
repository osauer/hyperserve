package main

import (
	"testing"
	"github.com/osauer/hyperserve/go"
)

func TestMCPDevOpsPreset(t *testing.T) {
	// Example 1: Basic MCP support (no built-in tools/resources)
	srv1, _ := hyperserve.NewServer(
		hyperserve.WithMCPSupport("MyApp", "1.0.0"),
	)
	// MCP is enabled but no built-in tools or resources
	_ = srv1

	// Example 2: MCP with full built-in tools and resources
	srv2, _ := hyperserve.NewServer(
		hyperserve.WithMCPSupport("MyApp", "1.0.0"),
		hyperserve.WithMCPBuiltinTools(true),
		hyperserve.WithMCPBuiltinResources(true),
	)
	// All built-in tools and resources are available
	_ = srv2

	// Example 3: MCP with Observability (minimal, secure monitoring)
	srv3, _ := hyperserve.NewServer(
		hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPObservability()),
	)
	// Only 3 essential observability resources, no tools, no sensitive data exposed
	_ = srv3

	// Example 4: MCP Observability with STDIO transport for Claude Desktop
	srv4, _ := hyperserve.NewServer(
		hyperserve.WithMCPSupport("MyApp", "1.0.0", 
			hyperserve.MCPOverStdio(),
			hyperserve.MCPObservability(),
		),
	)
	// Observability resources available via STDIO for Claude Desktop integration
	_ = srv4
}