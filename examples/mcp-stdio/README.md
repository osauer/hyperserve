# MCP Stdio Server for Claude Desktop

This example demonstrates a standalone MCP server that communicates via stdio, allowing Claude Desktop to interact with your local system through MCP tools and resources.

## Features

- **Stdio Communication**: Uses stdin/stdout for JSON-RPC 2.0 protocol
- **Sandboxed File Access**: Safe file operations within a designated directory
- **Built-in Tools**:
  - `calculator`: Basic math operations
  - `read_file`: Read files from sandbox
  - `list_directory`: List directory contents
- **Resources**: Sandbox information resource

## Building

```bash
# Build the executable
go build -o hyperserve-mcp-stdio

# Or install globally
go install github.com/osauer/hyperserve/examples/mcp-stdio@latest
```

## Claude Desktop Configuration

Add to your Claude Desktop configuration file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "hyperserve-local": {
      "command": "/path/to/hyperserve-mcp-stdio",
      "args": ["-sandbox", "/path/to/sandbox"]
    }
  }
}
```

### Example Configuration

```json
{
  "mcpServers": {
    "hyperserve-local": {
      "command": "/usr/local/bin/hyperserve-mcp-stdio",
      "args": ["-sandbox", "/Users/yourname/Documents/claude-sandbox", "-verbose"]
    }
  }
}
```

## Command Line Options

- `-sandbox <path>`: Set sandbox directory (default: `~/.hyperserve-mcp/sandbox`)
- `-verbose`: Enable verbose logging to stderr

## Usage in Claude

Once configured, you can use the tools in Claude:

```
"Use the calculator tool to compute 15 * 24"
"List the files in the sandbox directory"
"Read the contents of hello.txt"
```

## Security

- All file operations are restricted to the sandbox directory
- Path traversal attempts are blocked
- No network access or system commands

## How It Works

This is a **stdio server** - it communicates via standard input/output:
- **Reads** JSON-RPC requests from stdin
- **Writes** JSON-RPC responses to stdout  
- **Logs** to stderr (when using `-verbose`)

When you run it directly, it will appear to "hang" because it's waiting for input. This is normal!

## Testing

### Quick Test
```bash
# Single command test
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./hyperserve-mcp-stdio | jq

# Run the included test script
./test.sh
```

### Interactive Testing
```bash
# In one terminal, start the server with verbose logging
./hyperserve-mcp-stdio -verbose

# Type or paste JSON-RPC requests:
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculator","arguments":{"operation":"multiply","a":15,"b":4}}}

# Press Ctrl+D to exit
```

## Troubleshooting

1. **Check Claude Desktop logs**: Look for MCP connection errors
2. **Test manually**: Run the server with `-verbose` flag
3. **Verify path**: Ensure the command path in config is correct
4. **Permissions**: Make sure the executable has execute permissions