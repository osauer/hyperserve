HyperServe is a Go HTTP server library with in-process MCP for production APIs,
SSE transports, WebSocket flows, and assistant-inspectable services without a
sidecar.

## What's new in __VERSION__

__HIGHLIGHTS__

## Install or upgrade

~~~sh
go get github.com/osauer/hyperserve/v2@__VERSION__
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@__VERSION__
~~~

Library imports stay under:

~~~go
github.com/osauer/hyperserve/v2/pkg/server
github.com/osauer/hyperserve/v2/pkg/auth
github.com/osauer/hyperserve/v2/pkg/mcp
github.com/osauer/hyperserve/v2/pkg/jsonrpc
github.com/osauer/hyperserve/v2/pkg/websocket
~~~

See the [README](https://github.com/osauer/hyperserve/tree/__VERSION__#readme),
[v2 migration guide](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/MIGRATING_V2.md),
[API stability policy](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/API_STABILITY.md),
and [production guide](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/PRODUCTION.md)
for the supported v2 surface.

---
