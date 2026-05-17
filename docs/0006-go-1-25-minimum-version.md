# ADR-0006: Go 1.25 Minimum Version Requirement

**Status:** Accepted (revised 2026-05-17 for 0.25.0 release)
**Date:** 2024-12-01 (original); 2026-05-17 (bumped from 1.24 to 1.25)
**Deciders:** hyperserve team

## Context

HyperServe targets Go 1.25 as the minimum supported version. The 1.24 features remain load-bearing (`os.Root` for the static-file sandbox, Swiss-table maps for the rate limiter, FIPS-140-3 mode, X25519MLKEM768 post-quantum key exchange). 1.25 adds:

- **`sync.WaitGroup.Go(func())`** — replaces `Add(1)/defer Done()` ceremony; HyperServe uses it in the deferred-init lifecycle and the WebSocket pool maintenance loop.
- **`testing/synctest`** (stable) — virtual-time clocks for tests that previously needed real sleeps. Lets the SSE / deferred-init / rate-limiter-cleanup tests run faster and without flakes.
- **Container-aware `GOMAXPROCS`** — the scaffold's Distroless Dockerfile no longer needs to document the workaround.

## Decision

Require Go 1.25 as the minimum supported version. Continue to leverage:
- `os.Root` for the static-file sandbox.
- Swiss-table maps for the rate limiter.
- `WithFIPSMode()` for the FIPS-140-3 cipher restriction.
- Post-quantum X25519MLKEM768 key exchange by default.
- `sync.WaitGroup.Go(...)` over `Add(1)/Done()`.

## Consequences

### Positive
- Faster map operations (Swiss Tables, 1.24).
- `os.Root` sandboxing for the static-file server (1.24).
- FIPS-140-3 mode and X25519MLKEM768 post-quantum key exchange (1.24).
- `WaitGroup.Go(...)` removes lifecycle ceremony in concurrent code (1.25).
- `testing/synctest` makes time-sensitive tests deterministic (1.25 stable).

### Negative
- Drops support for Go 1.23 and earlier.
- CI/CD pipelines must be on 1.25+.

### Mitigation
- Documented in README + CHANGELOG.
- Migration guide explains the bump.

## Notes

The next bump (1.26) will be considered once `encoding/json/v2` graduates from experimental — chasing experimental stdlib APIs in a framework is bad citizenship for downstreams.
