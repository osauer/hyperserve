package main

import "testing"

func TestScaffoldMCPDefaultsOff(t *testing.T) {
	if defaultWithMCP {
		t.Fatal("MCP must remain explicit until the generated endpoint has an authentication policy")
	}
}
