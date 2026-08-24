# API Stability

_Last updated: 2026-08-24 (v2 line)._

## TL;DR

HyperServe is a **v2 Go module**. Public package APIs follow semantic
versioning:

- **PATCH (`2.x.y`)**: fixes and behavior corrections that preserve API shape.
- **MINOR (`2.x.0`)**: additive APIs and compatible behavior improvements.
- **MAJOR (`3.0.0`)**: breaking exported API changes and a `/v3` module path.

The v1 line remains available to existing importers. The v2 module path is the
source of truth on `main`; its deliberate breaking changes and direct
replacements are listed in [Migrating to v2](./MIGRATING_V2.md).

## Compatibility

The compatibility promise covers exported APIs under:

- `github.com/osauer/hyperserve/v2/pkg/server`
- `github.com/osauer/hyperserve/v2/pkg/auth`
- `github.com/osauer/hyperserve/v2/pkg/mcp`
- `github.com/osauer/hyperserve/v2/pkg/jsonrpc`
- `github.com/osauer/hyperserve/v2/pkg/websocket`

Examples, generated scaffold layout, and command packages are maintained as
release-gated developer experience, but they are not a stable import surface.
The repo is library-first; `cmd/hyperserve-init` is the supported command.

## Deprecation Policy

When an exported symbol needs to go away:

1. Prefer an additive replacement first.
2. Mark the old path as deprecated in Go docs.
3. Keep the deprecated path through the current major line unless it is unsafe
   or was explicitly removed by the v2 migration guide.
4. Remove it only in the next major module path.

Security fixes may change behavior when the old behavior is unsafe or
protocol-noncompliant, but those changes must be called out in the changelog.

## Release Gate

Releases are cut with `make release RELEASE_VERSION=vX.Y.Z`; do not tag,
push, or create the GitHub Release page by hand. The target requires a clean
tree, `HEAD == origin/main`, a non-existing tag, and a matching topmost
`CHANGELOG.md` entry before it tags anything.

Releases must pass:

- `make changelog-lint RELEASE_VERSION=vX.Y.Z`
- `go test ./...`
- `make check`
- standalone example-module checks
- canonical examples: `examples/devops`, `examples/mcp-extensions`, and
  `examples/json-api`
- deprecated transport compatibility example: `examples/mcp-sse`
- official MCP SDK conformance in the separated `tools` module
- scaffold generation plus `go test ./...` inside the generated project

GitHub Release notes are rendered from `.github/release-notes-template.md` plus
the matching `CHANGELOG.md` entry. The `### What's new` section is promoted to
the top of the release body, so there is one source of truth for release prose.

## Where To Look

- [CHANGELOG.md](../CHANGELOG.md) for release notes and migration notes.
- [ROADMAP.md](./ROADMAP.md) for planned work.
- GitHub Issues for concrete bugs.
- GitHub Discussions for API shape questions before a release locks the answer.
