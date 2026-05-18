# MCP with Server-Sent Events Example

This example demonstrates how to use HyperServe's MCP endpoint with Server-Sent Events (SSE) for real-time communication.

## Overview

HyperServe uses a unified endpoint approach - the same `/mcp` endpoint handles both regular HTTP requests and SSE connections based on the request headers.

## Running the Example

1. Start the server:
```bash
go run server.go
```

2. In another terminal, run the client:
```bash
go run client.go
```

## Key Points

- **Single Endpoint**: Both HTTP and SSE use `/mcp`
- **Header-Based Routing**: `Accept: text/event-stream` enables SSE
- **Client ID**: SSE clients receive a unique ID for request routing
- **Real-time**: Responses flow through the SSE stream

## Example Flow

1. Client connects with SSE header → receives client ID
2. Client sends requests with `X-SSE-Client-ID` header
3. Server processes requests and sends responses via SSE
4. Built-in keepalive maintains connection health

See the source files for implementation details.