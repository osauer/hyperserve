# Roadmap

This document records directions, not release promises. Issues and accepted
ADRs remain the authority for scheduled work.

## Current baseline

The v2.1 package reset establishes:

- the branded root `hyperserve` package;
- concern-specific `auth`, `jsonrpc`, `mcp`, `mcp/builtin`,
  `ratelimit`, and `websocket` packages;
- application-owned lifecycle and deterministic configuration binding;
- standalone, bounded rate-limit middleware;
- MCP 2026-07-28 Streamable HTTP plus stdio;
- RFC 6455 server and outbound-client support.

The baseline must settle before another public surface is added. Future
breaking changes require a new major module path.

## Near-term work

| Area | Direction | Constraint |
|---|---|---|
| MCP conformance | Track protocol revisions through focused fixtures and the official SDK suite. | Do not confuse proprietary routed SSE with current Streamable HTTP. |
| Authentication examples | Keep `auth` provider-neutral while maintaining at least one real provider integration example. | Identity must not imply application authorization. |
| Limiter operations | Add evidence only where operators need it, without exporting mutable quota internals. | Preserve bounded state, no cleanup goroutine, and explicit proxy trust. |
| Generated applications | Keep scaffold output compiling against the exact public release and application-owned policy. | Generated modules must contain no local replacement. |
| Performance | Maintain exact-revision A/B baselines for middleware, limiter, and loopback workloads. | No universal throughput claim from a microbenchmark. |
| Protocol hardening | Continue fuzzing WebSocket, JSON-RPC, MCP headers, and streaming cancellation. | Correctness and denial-of-service bounds precede convenience. |

## Deliberately out of scope

- ORM or database abstraction;
- browser-session management;
- identity-provider configuration inside the root package;
- application roles and resource authorization;
- automatic trust of `Forwarded` or `X-Forwarded-For`;
- a process-wide default limiter policy;
- a custom router solely for benchmark position;
- retaining duplicate public APIs for compatibility.

## How work enters the roadmap

A proposal should identify the user problem, authority boundary, public API
impact, expected dependency cost, and verification witness. Changes to package
direction or lifecycle require an ADR. Security-sensitive features need
fail-closed behavior and tests before they become examples or scaffold
defaults.
