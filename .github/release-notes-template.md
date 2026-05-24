HyperServe is a small, library-first Go HTTP framework with in-process MCP for
agentic workloads: production API servers, SSE transports, WebSocket flows, and
assistant-inspectable services without a sidecar.

## What's new in __VERSION__

__HIGHLIGHTS__

## Install or upgrade

~~~sh
go get github.com/osauer/hyperserve@__VERSION__
go install github.com/osauer/hyperserve/cmd/hyperserve-init@__VERSION__
~~~

Library imports stay under:

~~~go
github.com/osauer/hyperserve/pkg/server
github.com/osauer/hyperserve/pkg/mcp
github.com/osauer/hyperserve/pkg/jsonrpc
github.com/osauer/hyperserve/pkg/websocket
~~~

See the [README](https://github.com/osauer/hyperserve#readme), [API stability
policy](https://github.com/osauer/hyperserve/blob/main/docs/API_STABILITY.md),
and [production guide](https://github.com/osauer/hyperserve/blob/main/docs/PRODUCTION.md)
for the supported v1 surface.

---
