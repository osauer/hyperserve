# ADR-0013: Explicit Configuration Binding

**Status:** Accepted
**Date:** 2026-08-23
**Deciders:** hyperserve team

## Context

`NewServer` previously read `options.json`, `HS_CONFIG_PATH`, and supported
environment variables automatically. That was convenient for standalone
programs, but surprising for an embeddable library: an unrelated process
variable or working-directory file could enable listeners, MCP, CORS, or other
capabilities without appearing at the call site.

Applications also need two distinct forms of ownership. Some want a complete,
reviewed `Options` snapshot; others deliberately delegate selected values
to a config file or process environment.

## Decision

`NewServer` starts from deterministic built-in defaults and applies
`Option` values from left to right. It does not bind external
configuration implicitly.

- `WithOptions` replaces the current snapshot with a defensive copy.
- `WithConfigFile(path)` overlays fields from one application-chosen JSON file.
- `WithEnvironment()` overlays the documented environment variables and does
  not consult `HS_CONFIG_PATH`.
- Later options win, so applications can place deployment-owned inputs before
  invariants such as `WithAddr` or `WithMCPSupport`.

The former ambient-loading constructor is removed in v2. Callers must name the
file or environment source they intend to trust.

## Consequences

Construction is inspectable and safe to embed: the call site reveals which
external authorities may influence the server. Twelve-factor deployments remain
supported by adding `WithEnvironment()`. Existing applications that relied on
implicit files or environment variables must add the corresponding option.

The options are intentionally composable rather than introducing a second
constructor or a parallel configuration type. One resolution path initializes
logging, MCP, and filesystem roots only after the final snapshot is bound.
