# ADR-0018: One module and one reference application

Status: Accepted

Decision: Oliver, 2026-10-31 07:51 CET. Approved the proposed catalogue
limits and replacement surfaces, requested completion of the renovation,
and asked to simplify versioned imports for his single-owner application set.

## Decision

Use `github.com/osauer/hyperserve` without a major-version suffix. Go requires
`/vN` for module releases with major version N >= 2; the next unsuffixed release
is `v1.7.0`, following the existing `v1.6.1` tag. Preserve all old tags. This is
one evolving API with coordinated consumer migrations, not a promise that every
future change is source-compatible. See [API stability](API_STABILITY.md).

Retain the integration and ownership boundaries in
[ADR-0017](0017-v3-integrated-server.md), superseding its proposed `/v3` path.
Keep the standard `net/http` shapes and existing concern packages. Add no
runtime, frontend or test dependencies; `golang.org/x/time/rate` remains the
runtime module's sole external dependency. Developer tools and the OIDC example
remain isolated modules. Proprietary routed SSE remains opt-in compatibility.

The catalogue replaces `complete` and `best-practices`. Remove their hardcoded
feature cards, simulated CPU/Memory chart, random-error action and duplicate
About page. Show actual documents, stored bytes, uploads and connection state.
Keep separate focused examples for templates, configuration and rate limiting.

The example's policy file owns the approved limits: 10 MiB per file, 50 MiB of
stored file content, 100 documents, memory only and lost on restart. One upload
is admitted at a time; rejected uploads do not mutate the collection. The UI
reads the policy from the server. The public demonstration token is restricted
to numeric loopback listeners. A deployed application owns real credentials
and authorization.

## Verification and migration

Test actual application composition, rejected credentials at a relocated MCP
endpoint, shared HTTP/MCP data, admission and storage limits, live updates,
cancellation and shutdown. Inspect the desktop render and generated consumer.
Use disposable copies of Desk and Canary for import-path and integration checks;
library renovation does not update their shared checkouts or running services.

Run normal, race and fuzz gates on the final tree. Publication and fresh public
module retrieval remain separate from a local-main commit. Breaking changes
must name affected callers and migration instructions at release time.
