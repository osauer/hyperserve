# HyperServe API Specification

This document defines the API specification that HyperServe implements.

## Core Endpoints

### Health Checks
- `GET /healthz` - Returns 200 if server is healthy
- `GET /readyz` - Returns 200 if server is ready to accept requests  
- `GET /livez` - Returns 200 if server is alive

### MCP Endpoint
- `POST /mcp` - Model Context Protocol endpoint
- `GET /mcp` - Returns MCP capability information
- `GET /mcp` with `Accept: text/event-stream` - SSE connection

### Static Files
- `GET /static/*` - Serves files from configured static directory

## Response Formats

### Health Check Response
```json
{
  "status": "healthy",
  "uptime": 3600,
  "total_requests": 1234
}
```

### MCP Initialize Response
```json
{
  "jsonrpc": "2.0",
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {"listChanged": false},
      "resources": {"subscribe": false, "listChanged": false},
      "sse": {"enabled": true, "endpoint": "same", "headerRouting": true}
    },
    "serverInfo": {
      "name": "hyperserve",
      "version": "1.0.0"
    }
  },
  "id": 1
}
```

## Middleware Requirements

All implementations MUST provide these middleware in the default configuration:

1. **Request Logging**
   - Log format: `{timestamp} {ip} {method} {path} {status} {duration}`
   - Must log to stdout/structured logging

2. **Panic Recovery**
   - Catch panics and return 500
   - Log panic details with stack trace

3. **Security Headers**
   ```
   X-Content-Type-Options: nosniff
   X-Frame-Options: DENY
   Strict-Transport-Security: max-age=31536000
   Content-Security-Policy: [configurable]
   ```

4. **Rate Limiting**
   - Default: 100 req/s per IP, burst 200
   - Returns 429 when exceeded
   - Headers:
     - X-RateLimit-Limit
     - X-RateLimit-Remaining
     - X-RateLimit-Reset

5. **Metrics Collection**
   - Total requests
   - Response time (average, min, max)
   - Active WebSocket connections

## WebSocket Support

- Upgrade: `GET /ws` with proper headers
- Text and Binary frame support
- Ping/Pong handling (30s interval)
- Clean disconnection with close frames

## Configuration

Both implementations must support:

1. **Environment Variables**
   - `SERVER_ADDR` (default: `:8080`)
   - `HEALTH_ADDR` (default: `:9080`)
   - `HS_MCP_ENABLED` (default: `false`)
   - `HS_LOG_LEVEL` (default: `INFO`)
   - `HS_DEBUG` (default: `false`)

2. **Config File** 
   - `options.json` in working directory
   - JSON format with same field names

3. **Precedence**
   - CLI flags (if supported) > Env vars > Config file > Defaults

## ASCII Banner

Both implementations MUST display on startup:
```
 _                                              
| |__  _   _ _ __   ___ _ __ ___  ___ _ ____   _____
| '_ \| | | | '_ \ / _ \ '__/ __|/ _ \ '__\ \ / / _ \
| | | | |_| | |_) |  __/ |  \__ \  __/ |   \ V /  __/
|_| |_|\__, | .__/ \___|_|  |___/\___|_|    \_/ \___|
       |___/|_|     
```