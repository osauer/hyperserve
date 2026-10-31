# DevOps / observability MCP example

Shows the `MCPObservability()` preset — a curated namespace of read-only
introspection resources (process info, metrics snapshots, configuration view)
exposed via MCP so AI assistants can answer "what's this server doing?"
without an out-of-process bridge.

Read-only does not mean authenticated. The HTTP example binds to loopback;
an application exposing MCP elsewhere must add its own authentication and
authorization policy.

Runs in two modes:

- **HTTP transport** (default): MCP at `http://127.0.0.1:8080/mcp`. Useful
  for browser tools and `curl`-based exploration.
- **stdio transport** (`--mcp-stdio`): MCP on the process stdio loop. This
  is the shape Claude Desktop expects when you register a local MCP server.

## Run

```bash
# HTTP transport — MCP discovery + tools
go run ./examples/devops &
curl -s http://localhost:8080/.well-known/mcp.json | jq .

# stdio transport (for Claude Desktop registration)
go run ./examples/devops -- --mcp-stdio
```

## Notes

- Built-in MCP tools and resources are **off by default** in HyperServe.
  This example opts in via `MCPObservability()`, which registers the
  read-only observability resources. The preset does not authenticate callers.
- The `_ "github.com/osauer/hyperserve/v2/mcp/builtin"` blank import is
  required to register the preset hooks. Without it the preset is empty.
