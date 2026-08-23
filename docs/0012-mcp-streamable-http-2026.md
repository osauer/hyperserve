# ADR-0012: MCP 2026-07-28 Streamable HTTP

## Status

ACCEPTED

This ADR supersedes the HTTP/SSE transport portions of ADR-0011. Its tool,
resource, namespace, and security decisions remain in force.

## Context

MCP 2026-07-28 replaces session-oriented server push with stateless POST
requests and the request-scoped `subscriptions/listen` RPC. HyperServe already
served finite 2026 requests as JSON, but its live-update path was a proprietary
GET stream routed by `X-SSE-*` headers. Advertising that transport by default
made standard and compatibility behavior easy to confuse.

HyperServe must support live resource invalidations without adding protocol
sessions, replay state, or a second public subscription abstraction. It must
also preserve a bounded migration path for existing HyperServe-specific
clients.

## Decision

- `/mcp` serves MCP 2026-07-28 as stateless POST requests. Finite RPCs return
  JSON; `subscriptions/listen` returns request-scoped SSE.
- The listen request requires an ID and `notifications` object. HyperServe
  supports `resourceSubscriptions` through the existing
  `SubscribableResourceTemplate` and `ResourceEmitter` interfaces, coalesces
  duplicate URIs, and accepts at most 128 URI entries.
- The first SSE message is `notifications/subscriptions/acknowledged` and
  contains only matched URIs. Every later update carries the listen request ID
  as `io.modelcontextprotocol/subscriptionId`.
- One goroutine owns response writes. Producers use a cancellation-aware,
  bounded 32-event queue and block instead of dropping events. Writes have a
  30-second deadline; 30-second SSE comments keep idle connections alive.
- Listen streams do not emit event IDs, resumability state, protocol pings, or
  progress notifications. Closing the response cancels its producers and
  forbids later writes.
- Natural producer completion and `Handler.Shutdown` send a final
  `resultType: complete` response. Server shutdown asks the MCP handler to
  complete streams before canceling the HTTP base context.
- The initialize-era 2025-11-25 request/response path remains as an explicit
  compatibility fallback without sessions or resumability. Its configured
  version cannot be set to the current 2026 revision.
- HyperServe's proprietary routed GET SSE is deprecated and disabled by
  default. `WithMCPLegacyRoutedSSE(true)` or
  `Handler.SetLegacyRoutedSSEEnabled(true)` temporarily restores it. Discovery
  advertises it only when enabled.
- Modern parsing rejects duplicate JSON keys, ambiguous singleton headers,
  response envelopes, invalid IDs, cross-origin requests, oversized bodies,
  malformed media types, and legacy/current transport confusion before RPC
  dispatch.

## Consequences

Current MCP clients get a standard live-update path with bounded memory and
deterministic shutdown. Existing `SubscribableResourceTemplate`
implementations need no code changes. Existing HyperServe-specific routed-SSE
clients must opt in while they migrate.

The shipped module keeps its single runtime dependency. The official MCP Go
SDK v1.7.0 is pinned only in the separate `tools` module for discovery, tool,
subscription, update, and cancellation conformance tests.

MRTR, protocol sessions, resumability, prompts, and a public progress API are
outside this decision.

## References

- [MCP 2026-07-28 Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)
- [ADR-0011](./0011-mcp-protocol-support.md)
- [MCP integration guide](./MCP_GUIDE.md)
