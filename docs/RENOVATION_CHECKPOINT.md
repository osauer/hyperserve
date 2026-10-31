# Renovation handoff

2026-10-31 08:27 CET. Original base: local main
`ba983b3131d428520d0ebe1861aad83e4d95f57e` (v2.2.1).

The current decision is [ADR-0019](0019-v2-maintenance.md): retain `/v2`, normal
semantic versioning, the catalogue and its approved limits, near-zero
dependencies, and isolated routed-SSE compatibility. The unpublished module-path
proposals are historical decisions, not consumer migration instructions.

Prepare v2.2.2 through `make release-smoke RELEASE_VERSION=v2.2.2`,
`make test-race`, `make fuzz-smoke`, disposable Desk consumer tests and browser
smoke tests. Use `make release RELEASE_VERSION=v2.2.2` for the authorized
publication; bind final evidence to the committed tree and verify fresh public
module retrieval separately. Do not update or restart shared consumer services.

The catalogue replaces the approved old surfaces. Current upgrade details are
in [Migration](MIGRATING.md); historical ADRs and release entries remain intact.
A local replacement proves integration, not publication. Docker execution and
GitHub branch-protection changes remain outside the release evidence.
