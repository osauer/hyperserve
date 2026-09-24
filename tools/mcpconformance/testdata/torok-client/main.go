// This consumer is built only by scripts/check-torok-consumers.sh, against
// immutable local archives. Torok is not a dependency of the public test module.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/osauer/torok/tool/mcp"
)

func main() {
	if len(os.Args) != 2 {
		panic("expected synthetic server executable")
	}
	for _, version := range []string{"2025-11-25", "2025-06-18"} {
		if err := check(os.Args[1], version); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func check(server, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mcp.New(ctx, mcp.Config{
		Command: server, Args: []string{"-protocol-version", version},
		ReadOnly: []string{"echo", "fail"}, Timeout: 5 * time.Second,
	})
	if err != nil {
		if version == "2025-11-25" && strings.Contains(err.Error(), "unsupported MCP protocol version") {
			fmt.Printf("%s: client requires the explicit compatibility profile\n", version)
			return nil
		}
		return err
	}
	defer client.Close()
	tools, err := client.Tools(ctx)
	if err != nil {
		return err
	}
	if len(tools) != 2 {
		return fmt.Errorf("got %d tools, want 2", len(tools))
	}
	for _, tool := range tools {
		switch tool.Spec().Name {
		case "echo":
			result, err := tool.Call(ctx, json.RawMessage(`{"message":"synthetic-hello"}`))
			if err != nil {
				return err
			}
			var envelope struct {
				Content           []struct{ Type, Text string }
				StructuredContent struct{ Message string }
				IsError           bool
			}
			if err := json.Unmarshal(result, &envelope); err != nil {
				return err
			}
			if envelope.IsError || len(envelope.Content) != 1 || envelope.Content[0].Type != "text" || envelope.StructuredContent.Message != "synthetic-hello" {
				return fmt.Errorf("unexpected echo: %s", result)
			}
			var text struct{ Message string }
			if err := json.Unmarshal([]byte(envelope.Content[0].Text), &text); err != nil || text.Message != "synthetic-hello" {
				return fmt.Errorf("unexpected text: %s", result)
			}
		case "fail":
			if _, err := tool.Call(ctx, json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "synthetic rejection") {
				return fmt.Errorf("domain error = %v", err)
			}
		default:
			return fmt.Errorf("unexpected tool %s", tool.Spec().Name)
		}
	}
	fmt.Printf("%s: discovery, structured output, text output, and domain errors passed\n", version)
	return nil
}
