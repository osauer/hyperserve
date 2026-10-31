# Renovation handoff

2026-10-31 08:04 CET. Base: local main
`ba983b3131d428520d0ebe1861aad83e4d95f57e`.

The accepted decisions are recorded in [ADR-0018](0018-single-owner-module.md):
no module-path suffix, one catalogue reference application, owner-approved
storage limits, near-zero dependencies, and isolated routed-SSE compatibility.
No owner decisions remain open for this local renovation.

The catalogue replaces `complete` and `best-practices`. Current import and
behavior changes are in [Migration](MIGRATING.md). Historical ADRs and release
entries retain their original text; the changelog preamble points to current
policy. No new release entry or tag is part of this renovation.

Consumer checks use committed, disposable copies of Desk
`1e3b264832ad56c47d608520a1c1b64ef0786b05` and Canary
`ebb0c57ed8a034d3225e83b1d4087357fc872c6f`. Only HyperServe imports and module
selection change in those copies. Desk's streaming and server-lifetime tests,
and Canary's HTTP configuration/authentication, SSE and WebSocket tests pass.
The shared consumer checkouts and running applications remain untouched.

Final local gates: `make release-smoke RELEASE_VERSION=v1.7.0`,
`make test-race`, and `make fuzz-smoke`. Bind their results to the committed
source tree in the completion evidence. The generated consumer uses a local
replacement; it does not establish public retrieval.

Publication, hosted CI on the final commit, Docker execution, real consumer
migration and GitHub branch-protection changes are separate external actions.
