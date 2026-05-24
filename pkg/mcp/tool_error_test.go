package mcp

import (
	"context"
	"fmt"
	"testing"

	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

func TestToolErrorReturnsMCPIsErrorResult(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterTool(NewTool("gateway").
		WithExecute(func(map[string]any) (any, error) {
			return nil, ToolError("gateway unavailable")
		}).
		Build())

	result, err := h.handleToolsCall(map[string]any{
		"name":      "gateway",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("handleToolsCall returned JSON-RPC error: %v", err)
	}
	toolResult := result.(ToolResult)
	if !toolResult.IsError {
		t.Fatal("ToolResult.IsError = false, want true")
	}
	if got := toolResult.Content[0]["text"]; got != "gateway unavailable" {
		t.Fatalf("tool error text = %v, want gateway unavailable", got)
	}
}

func TestToolErrorfWorksForTypedTools(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterTool(NewTypedTool("typed_gateway", "",
		func(context.Context, struct{}) (struct{}, error) {
			return struct{}{}, ToolErrorf("gateway %s", "unavailable")
		}))

	result, err := h.handleToolsCall(map[string]any{
		"name":      "typed_gateway",
		"arguments": map[string]any{},
	})
	if err != nil {
		t.Fatalf("handleToolsCall returned JSON-RPC error: %v", err)
	}
	toolResult := result.(ToolResult)
	if !toolResult.IsError || toolResult.Content[0]["text"] != "gateway unavailable" {
		t.Fatalf("ToolResult = %+v, want isError gateway message", toolResult)
	}
}

func TestOrdinaryToolErrorsRemainJSONRPCErrors(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterTool(NewTool("plain_error").
		WithExecute(func(map[string]any) (any, error) {
			return nil, fmt.Errorf("plain failure")
		}).
		Build())

	_, err := h.handleToolsCall(map[string]any{
		"name":      "plain_error",
		"arguments": map[string]any{},
	})
	if err == nil {
		t.Fatal("ordinary error was converted to ToolResult, want handler error")
	}
}

func TestUnknownToolStillReturnsJSONRPCError(t *testing.T) {
	h := newHandlerForTest(t)
	resp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "tools/call",
		Params:  map[string]any{"name": "missing", "arguments": map[string]any{}},
		ID:      1,
	})
	if resp.Error == nil {
		t.Fatal("unknown tool returned success, want JSON-RPC error")
	}
	if resp.Result != nil {
		t.Fatalf("unknown tool result = %v, want nil", resp.Result)
	}
}
