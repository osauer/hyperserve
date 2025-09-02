# WebSocket Connection Pool Example

This example demonstrates HyperServe's WebSocket connection pooling feature, which provides efficient connection reuse and resource management for WebSocket applications.

## Features

- **Connection Pooling**: Reuse existing WebSocket connections to reduce establishment overhead
- **Health Monitoring**: Automatic ping/pong health checks to detect dead connections
- **Idle Connection Cleanup**: Configurable timeout for unused connections
- **Pool Statistics**: Real-time metrics on connection usage and health
- **Configurable Limits**: Per-endpoint connection limits and pool size controls

## Running the Example

```bash
cd examples/websocket-pool
go run main.go
```

Open http://localhost:8085 in your browser to access the interactive demo.

## Key Components

### WebSocketPool Configuration

```go
poolConfig := hyperserve.PoolConfig{
    MaxConnectionsPerEndpoint: 100,    // Max connections per endpoint
    MaxIdleConnections:        20,     // Max idle connections to keep
    IdleTimeout:              30 * time.Second,
    HealthCheckInterval:      10 * time.Second,
    ConnectionTimeout:        5 * time.Second,
    EnableCompression:        true,
}

pool := hyperserve.NewWebSocketPool(poolConfig)
```

### Using the Pool

```go
// Get connection from pool or create new one
conn, err := pool.Get(r.Context(), endpoint, upgrader, w, r)
if err != nil {
    return err
}

// Return connection to pool when done
defer pool.Put(conn)
```

## Pool Statistics

The pool provides real-time statistics:

- **Total Connections**: All connections in the pool
- **Active Connections**: Currently in-use connections
- **Idle Connections**: Available for reuse
- **Connections Created**: Total new connections made
- **Connections Reused**: Connections taken from pool
- **Health Checks Failed**: Failed ping/pong checks

## Use Cases

1. **High-Traffic Applications**: Reduce connection overhead under load
2. **Real-time Services**: Maintain persistent connections efficiently  
3. **IoT Gateways**: Handle many concurrent device connections
4. **Chat Applications**: Manage user connections with automatic cleanup
5. **Live Data Feeds**: Optimize resource usage for streaming services

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `MaxConnectionsPerEndpoint` | Maximum connections per endpoint | 10 |
| `MaxIdleConnections` | Maximum idle connections to retain | 5 |
| `IdleTimeout` | Time before idle connections are closed | 30s |
| `HealthCheckInterval` | Frequency of ping/pong health checks | 10s |
| `ConnectionTimeout` | Timeout for new connection establishment | 10s |
| `EnableCompression` | Enable WebSocket compression | false |

## Best Practices

1. **Set Appropriate Limits**: Configure based on your expected load
2. **Monitor Statistics**: Use pool stats to optimize configuration
3. **Handle Errors Gracefully**: Pool operations can fail under high load
4. **Clean Shutdown**: Always call `pool.Shutdown()` on application exit
5. **Health Check Tuning**: Adjust interval based on connection quality needs