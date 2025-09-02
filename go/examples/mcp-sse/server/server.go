package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/osauer/hyperserve/go"
)

// CustomTool demonstrates a simple MCP tool
type CustomTool struct{}

func (t *CustomTool) Name() string        { return "echo" }
func (t *CustomTool) Description() string { return "Echoes the input message" }
func (t *CustomTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Message to echo",
			},
		},
		"required": []string{"message"},
	}
}

func (t *CustomTool) Execute(params map[string]interface{}) (interface{}, error) {
	message, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message parameter is required")
	}
	return fmt.Sprintf("Echo: %s", message), nil
}

func main() {
	// Create server with MCP support
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithMCPSupport("sse-example", "1.0.0"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Register a custom tool
	srv.RegisterMCPTool(&CustomTool{})

	// Add a regular HTTP endpoint
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "HyperServe MCP + SSE Example")
		fmt.Fprintln(w, "MCP endpoint: /mcp")
		fmt.Fprintln(w, "- HTTP: POST /mcp")
		fmt.Fprintln(w, "- SSE: GET /mcp with Accept: text/event-stream")
	})

	log.Println("Server starting on :8080")
	log.Println("MCP endpoint available at http://localhost:8080/mcp")
	srv.Run()
}