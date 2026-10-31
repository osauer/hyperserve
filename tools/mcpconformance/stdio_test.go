package mcpconformance

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	hyperservemcp "github.com/osauer/hyperserve/v2/mcp"
)

func TestOfficialSDKStdio(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "stdio-server")
	build := exec.Command("go", "build", "-o", binary, "./testdata/stdio-server")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}
	for _, version := range []string{hyperservemcp.DefaultProtocolVersion, "2025-06-18"} {
		t.Run(version, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "stdio-check", Version: "1"}, nil)
			session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: exec.Command(binary, "-protocol-version", version)}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			if initialized := session.InitializeResult(); initialized == nil || initialized.ProtocolVersion != version {
				t.Fatalf("initialize = %+v, want version %s", initialized, version)
			}
			listed, err := session.ListTools(ctx, nil)
			if err != nil || len(listed.Tools) != 2 {
				t.Fatalf("tools = %+v, error = %v", listed, err)
			}
			for _, tool := range listed.Tools {
				if tool.OutputSchema == nil {
					t.Fatalf("missing output schema for %s", tool.Name)
				}
			}
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "synthetic-hello"}})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError || len(result.Content) != 1 {
				t.Fatalf("echo = %+v", result)
			}
			text, ok := result.Content[0].(*sdkmcp.TextContent)
			if !ok {
				t.Fatalf("echo content = %T", result.Content[0])
			}
			var echoed map[string]string
			if err := json.Unmarshal([]byte(text.Text), &echoed); err != nil || echoed["message"] != "synthetic-hello" {
				t.Fatalf("text = %q, error = %v", text.Text, err)
			}
			structured, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			echoed = nil
			if err := json.Unmarshal(structured, &echoed); err != nil || echoed["message"] != "synthetic-hello" {
				t.Fatalf("structured = %s, error = %v", structured, err)
			}
			failed, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "fail", Arguments: map[string]any{}})
			if err != nil || failed == nil || !failed.IsError {
				t.Fatalf("domain error = %+v, protocol error = %v", failed, err)
			}
		})
	}
}
