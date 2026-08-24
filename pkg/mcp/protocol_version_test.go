package mcp

import (
	"testing"

	jsonrpc "github.com/osauer/hyperserve/v2/pkg/jsonrpc"
)

func TestHandlerProtocolVersionDefaultAndOverride(t *testing.T) {
	h := newHandlerForTest(t)
	if h.ProtocolVersion() != DefaultProtocolVersion {
		t.Fatalf("ProtocolVersion = %q, want %q", h.ProtocolVersion(), DefaultProtocolVersion)
	}

	resp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		},
		ID: 1,
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	if result["protocolVersion"] != DefaultProtocolVersion {
		t.Fatalf("default initialize protocolVersion = %v, want %s", result["protocolVersion"], DefaultProtocolVersion)
	}

	h.SetProtocolVersion("2025-06-18")
	resp = h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "initialize",
		ID:      2,
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	result = resp.Result.(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("custom initialize protocolVersion = %v, want 2025-06-18", result["protocolVersion"])
	}

	h.SetProtocolVersion("")
	if h.ProtocolVersion() != DefaultProtocolVersion {
		t.Fatalf("ProtocolVersion after reset = %q, want %q", h.ProtocolVersion(), DefaultProtocolVersion)
	}
}

func TestDiscoveryUsesConfiguredProtocolVersion(t *testing.T) {
	h := newHandlerForTest(t)
	h.SetProtocolVersion("2025-06-18")

	info := h.BuildDiscoveryInfo(newDiscoveryRequest(""), DiscoveryConfig{
		MCPEndpoint: "/mcp",
		DefaultAddr: ":8080",
		Transport:   HTTPTransport,
		Policy:      DiscoveryPublic,
	})
	if info.Version != StreamableHTTPProtocolVersion || len(info.Versions) != 1 {
		t.Fatalf("default discovery versions = %q/%v, want current only", info.Version, info.Versions)
	}

	h.SetLegacyRoutedSSEEnabled(true)
	info = h.BuildDiscoveryInfo(newDiscoveryRequest(""), DiscoveryConfig{MCPEndpoint: "/mcp"})
	if len(info.Versions) != 2 || info.Versions[1] != "2025-06-18" {
		t.Fatalf("legacy discovery versions = %v, want configured initialize version", info.Versions)
	}
}
