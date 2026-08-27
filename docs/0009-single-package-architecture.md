# ADR-0009: Single Package Architecture (SUPERSEDED)

**Status:** Superseded by [ADR-0014](./0014-root-package-and-concern-subpackages.md). The interim layered `pkg/` decision remains recorded below.
**Date:** 2024-12-01
**Deciders:** hyperserve team

## Summary

The original ADR mandated a single-package layout. That decision has been reversed:

- 0.21 (commit `c54c61b`) introduced `pkg/server`, `pkg/websocket`, `pkg/jsonrpc`.
- 0.22 (commit `fb70302`) dropped root facades, requiring `import "github.com/osauer/hyperserve/pkg/server"` on the v1 module line.
- 0.25 introduced `pkg/mcp` and `pkg/mcp/builtin`, moving MCP out of `pkg/server` to make the MCP differentiator visible from the import path and to cap `pkg/server` LOC.
- v2 keeps that package layout under the `github.com/osauer/hyperserve/v2`
  module path and adds the provider-neutral `pkg/auth` boundary.

The current layout is documented in [ADR-0014](./0014-root-package-and-concern-subpackages.md) and [ARCHITECTURE.md](../ARCHITECTURE.md). This file remains as a marker so the historical context is not lost.
