# ADR-0017: Integrated server contracts for v3

Status: Accepted

Decision: Oliver, 2026-10-30 18:42 CET; conversation choices 1B, 2A, 3A.

## Decision

Use the `github.com/osauer/hyperserve/v3` module path. Preserve the standard
`net/http` handler and middleware shapes and the existing concern packages.
Full vertical integration and near-zero dependencies are product constraints.
Keep `golang.org/x/time/rate`; add no runtime, frontend, or test dependencies.
Existing isolated developer tooling and the provider integration example remain.

HTTP, MCP, SSE, JSON-RPC and WebSocket remain implemented together here. The
application owns policy, data, credentials, context, and signals. HyperServe
owns the configured transport resources and their shutdown. Fewer dependencies
reduce supply-chain assembly; protocol correctness still needs our own tests.

MCP routes must compose with method-aware HTTP catch-all routes. Protection
must follow the resolved endpoint. Builtins remain opt-in. Retain proprietary
routed SSE only as explicitly selected compatibility behavior, per Oliver's
follow-up decision; it is not the current protocol.

Consolidate `complete` and `best-practices` into one document-catalogue reference
application. HTML, JSON, MCP and live updates expose the same application data.
Retain focused examples for separate lessons. Authentication and authorization
stay visible in the application rather than being inferred from discovery.

## Evidence required

Exercise startup, rejected credentials, configured endpoint changes, shared
HTTP/MCP data, streaming cancellation and shutdown. Static indexes, multipart
fields and non-destructive confined sample initialization have regression tests.
Multipart parsing retains the standard form parser's existing 10 MiB body
ceiling, including file parts, to bound temporary-file writes. This is a parser
contract, distinct from application upload/storage policy.
Use independent temporary directories for parallel tests. Run the repository
normal, race and fuzz gates, and inspect the reference app in a desktop browser.
A generated consumer must build from the candidate module; public retrieval is
separate evidence available only after an authorized release.

## Historical decisions

This decision and ARCHITECTURE.md are the current integration contract. ADRs
0001–0016 retain historical text and context; use the current API reference for
signatures and current guides for commands. This supersedes unsupported claims
about binary size, compilation time and middleware incompatibility in ADR-0001,
and old constructor examples in ADR-0008 without rewriting their history.
