// Smallest MCP-enabled HyperServe binary: built-in MCP tools/resources,
// a custom tool, a custom resource, and a sandboxed file-tool root.
//
//	go run ./examples/mcp-basic
//
// Hit the endpoint with:
//
//	curl -X POST http://localhost:8080/mcp \
//	  -H "Content-Type: application/json" \
//	  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/osauer/hyperserve/v2/pkg/mcp"
	_ "github.com/osauer/hyperserve/v2/pkg/mcp/builtin" // wire built-in tool/resource hooks
	serverpkg "github.com/osauer/hyperserve/v2/pkg/server"
)

// TimestampTool returns the current time in a caller-selected format.
type TimestampTool struct{}

func (TimestampTool) Name() string { return "timestamp" }
func (TimestampTool) Description() string {
	return "Return the current time in unix | iso8601 | rfc3339 format."
}
func (TimestampTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"format": map[string]any{
				"type":        "string",
				"description": "Output format",
				"enum":        []string{"unix", "iso8601", "rfc3339"},
				"default":     "rfc3339",
			},
		},
	}
}
func (TimestampTool) Execute(params map[string]any) (any, error) {
	format, _ := params["format"].(string)
	now := time.Now().UTC()
	switch format {
	case "unix":
		return now.Unix(), nil
	case "iso8601":
		return now.Format("2006-01-02T15:04:05Z"), nil
	case "", "rfc3339":
		return now.Format(time.RFC3339), nil
	default:
		return nil, fmt.Errorf("unknown format %q (expected unix|iso8601|rfc3339)", format)
	}
}

// ServerStatusResource exposes a tiny status blob via the MCP resource API.
type ServerStatusResource struct{ start time.Time }

func (r *ServerStatusResource) URI() string         { return "custom://server/status" }
func (r *ServerStatusResource) Name() string        { return "Server status" }
func (r *ServerStatusResource) Description() string { return "Process uptime and a static OK flag." }
func (r *ServerStatusResource) MimeType() string    { return "application/json" }
func (r *ServerStatusResource) Read() (any, error) {
	return map[string]any{
		"ok":             true,
		"uptime_seconds": time.Since(r.start).Round(time.Second).Seconds(),
	}, nil
}
func (r *ServerStatusResource) List() ([]string, error) { return []string{r.URI()}, nil }

var _ mcp.Tool = TimestampTool{}
var _ mcp.Resource = (*ServerStatusResource)(nil)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv, err := serverpkg.NewServer(
		serverpkg.WithAddr(":8080"),
		serverpkg.WithRateLimit(50, 100),
		serverpkg.WithEnvironment(), // Deployment may override address, endpoint, and rate.

		// Keep application-owned MCP capabilities explicit and later in the chain.
		serverpkg.WithMCPSupport("mcp-basic", "1.0.0"),
		serverpkg.WithMCPBuiltinTools(true),
		serverpkg.WithMCPBuiltinResources(true),
		// Confine builtin file tools to ./sandbox via os.Root. Without this,
		// file tools refuse to construct.
		serverpkg.WithMCPFileToolRoot("examples/mcp-basic/sandbox"),
		serverpkg.WithTemplateDir("examples/mcp-basic/templates"),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := srv.RegisterMCPTool(TimestampTool{}); err != nil {
		log.Fatal(err)
	}
	if err := srv.RegisterMCPResource(&ServerStatusResource{start: time.Now()}); err != nil {
		log.Fatal(err)
	}

	// Template-rendered dashboard. Data func runs on every request.
	start := time.Now()
	if err := srv.HandleFuncDynamic("/", "index.html", func(r *http.Request) any {
		return map[string]any{
			"MCPEndpoint": srv.Options().MCPEndpoint,
			"SandboxDir":  srv.Options().MCPFileToolRoot,
			"Uptime":      time.Since(start).Round(time.Second).String(),
		}
	}); err != nil {
		log.Fatal(err)
	}

	log.Println("mcp-basic listening on http://localhost:8080")
	log.Println("MCP endpoint: http://localhost:8080/mcp (POST JSON-RPC)")
	log.Println("Dashboard:    http://localhost:8080/")
	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
