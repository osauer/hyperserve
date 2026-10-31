# ADR-0019: Preserve v2 compatibility through the renovation

Status: Accepted

Decision: Oliver, 2026-10-31 08:27 CET: restore `/v2` and normal semantic
versioning, resolve the two compatibility cases without surprises, retain the
renovation and prepare a maintenance release. Publication is authorized after
Desk integration and edge-case smoke tests.

## Decision

Keep `github.com/osauer/hyperserve/v2` and release the maintenance work as
`v2.2.2`. Supersede the module-path and compatibility-policy portions of
[ADR-0018](0018-single-owner-module.md) and the proposed v3 path in
[ADR-0017](0017-v3-integrated-server.md). Neither proposal was published.
Public Go APIs remain unchanged. Follow [API stability](API_STABILITY.md).

Retain the existing root and concern packages, standard net/http shapes,
application-owned authority and lifetime, and optional routed-SSE compatibility.
Retain ADR-0018's catalogue, approved policy file and replacement of the old
example surfaces. Add no dependencies. The catalogue uses one coherent
workbench for uploading, finding and downloading sample documents, with
executable JSON/MCP examples and live changes to the same collection.

## Compatibility mechanisms

BindForm previously called ParseForm without consuming multipart bodies.
Automatically parsing and capping them changed application ownership. Restore
the existing behavior and document the caller's explicit ParseMultipartForm
step; retain caller-selected limits and file access. No library limit is
loosened relative to released v2. The application still bounds multipart body
size and memory/disk use; without an application body limit, multipart file
storage has no total-byte ceiling. The catalogue retains its separate approved
raw-body limits and admits only one upload at a time.

Method-qualified MCP routes allowed unsupported methods to reach an application
catch-all. Restore methodless registrations and the protocol's own rejection.
Use exact homepage and asset routes in the catalogue instead of changing
ServeMux's public specificity rules. Preserve explicit application method
routes, middleware, relocated endpoints and extension HTTP method rejection.

## Evidence

Fail-before regression tests cover untouched multipart bodies, caller-parsed
large files and field precedence; MCP method rejection and explicit method
route precedence; and exact homepage composition. Run the release, race and
fuzz gates, disposable Desk integration, browser task inspection and independent
cold read. Publication and fresh-module retrieval are separate evidence.
