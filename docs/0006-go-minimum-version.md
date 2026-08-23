# ADR-0006: Go Minimum Version Policy

**Status:** Accepted (revised 2026-08-23 for the Go 1.27 bump)
**Date:** 2024-12-01 (original 1.21 floor); 2026-08-23 (1.26 → 1.27)
**Deciders:** hyperserve team

## Context

HyperServe pins its minimum Go version in `go.mod`. That file is the single
source of truth — docs, ADRs, and READMEs do not repeat the version, they
reference `go.mod`. Bumps land when a new release actually unlocks something
we use.

Current floor: **Go 1.27**.

Load-bearing features by minor:

- **1.22** — `for i := range N` loop form.
- **1.24** — `os.Root` (static-file sandbox), Swiss-table maps (rate
  limiter), FIPS-approved TLS cipher list, X25519MLKEM768 post-quantum key
  exchange.
- **1.25** — `sync.WaitGroup.Go(...)` (deferred-init lifecycle), stable
  `testing/synctest`, container-aware `GOMAXPROCS`.
- **1.26** — aligns the project with the current stable toolchain; the
  `encoding/json/v2` graduation will be tracked when it lands.
- **1.27** — current standard-library and language idioms enforced by the
  repository's native `go fix` and modernize workflow.

## Decision

The `go.mod` `go` directive is the authoritative floor. CI installs a Go
version that matches it. The scaffold's `go.mod.tmpl` follows the same floor
so generated projects compile against the same toolchain features HyperServe
itself uses.

Cadence: bump when a new minor unlocks a feature we adopt, or when staying
within the current minor blocks a useful subtraction. Don't bump
speculatively.

## Consequences

### Positive
- One place (`go.mod`) records the truth; no doc drift.
- Modern idioms (`range N`, `WaitGroup.Go`, `os.Root`) become available
  without per-version conditional code.

### Negative
- Each bump drops users on older toolchains.
- Tooling pinned in the `tool` directive (modernize) follows the same floor
  and must be re-resolved on bump.

### Mitigation
- CHANGELOG calls out the bump as a breaking-change line.
- CI builds on the floor only; we do not advertise a broader compatibility
  matrix than what we test.
