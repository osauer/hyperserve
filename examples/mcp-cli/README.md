# Application-owned MCP flags

Despite the historical directory name, this is an MCP **server**, not a client.
It shows how an application can translate its own command-line flags into
HyperServe options. HyperServe itself does not parse flags.

Run the HTTP server without MCP:

```bash
go run ./examples/mcp-cli
```

Enable the local-development preset over Streamable HTTP:

```bash
go run ./examples/mcp-cli --mcp-dev
```

Or expose the same server over stdio for a process-supervised MCP host:

```bash
go run ./examples/mcp-cli --mcp-dev --mcp-transport=stdio
```

The application accepts:

| Flag | Purpose |
|---|---|
| `--mcp-dev` | Local route inspection, server status, and developer help |
| `--mcp-observability` | Read-only server health and telemetry resources |
| `--mcp-transport=http\|stdio` | Transport used when a preset is enabled |
| `--port=8080` | Optional programmatic override for the listen port |

The important boundary is visible in the code:

```go
if *mcpDev {
    configs = append(configs, hyperserve.MCPDev())
}
opts = append(opts, hyperserve.WithMCPSupport("MyApp", "1.0.0", configs...))
```

The example explicitly starts with `WithEnvironment()`, so environment
configuration remains available when flags do not override it:

```bash
HS_MCP_ENABLED=true \
HS_MCP_DEV=true \
HS_MCP_SERVER_NAME=MyApp \
go run ./examples/mcp-cli
```

For stdio, build a stable executable path for the host configuration:

```bash
go build -o mcp-flags ./examples/mcp-cli
```

```json
{
  "mcpServers": {
    "myapp": {
      "command": "/absolute/path/to/mcp-flags",
      "args": ["--mcp-dev", "--mcp-transport=stdio"]
    }
  }
}
```

Do not expose the development preset on an untrusted network. It exposes route
and middleware layout, runtime status, and development logs. Prefer the
observability preset when an operator only needs read-only status.
