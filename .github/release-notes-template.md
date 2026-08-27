HyperServe __VERSION__ is a Go HTTP server library with in-process MCP,
WebSocket, SSE, typed input, and standard `net/http` handlers.

> [!WARNING]
> v2.1.0 is a controlled breaking reset inside the existing `/v2` module
> path. This is outside ordinary semantic-version compatibility. Read the
> [v2.1 migration guide](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/MIGRATING_V2_1.md)
> before upgrading. To roll back, pin
> `github.com/osauer/hyperserve/v2@v2.0.3`. Future breaking changes require a
> new major module path.

## What's new in __VERSION__

__HIGHLIGHTS__

## Install or upgrade

~~~sh
go get github.com/osauer/hyperserve/v2@__VERSION__
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@__VERSION__
~~~

Canonical imports:

~~~go
github.com/osauer/hyperserve/v2
github.com/osauer/hyperserve/v2/auth
github.com/osauer/hyperserve/v2/jsonrpc
github.com/osauer/hyperserve/v2/mcp
github.com/osauer/hyperserve/v2/mcp/builtin
github.com/osauer/hyperserve/v2/ratelimit
github.com/osauer/hyperserve/v2/websocket
~~~

See the [README](https://github.com/osauer/hyperserve/tree/__VERSION__#readme),
[v1 migration guide](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/MIGRATING_V2.md),
[API stability policy](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/API_STABILITY.md),
and [production guide](https://github.com/osauer/hyperserve/blob/__VERSION__/docs/PRODUCTION.md).

## Maintainer recovery

If the annotated tag push succeeds but GitHub Release creation fails, do not
move or replace the tag. First verify that the local and remote annotated tags
resolve to the same immutable commit and that the push CI run for that exact SHA
succeeded. Then recover only the publication step:

~~~sh
make release-publish RELEASE_VERSION=__VERSION__
~~~

---
