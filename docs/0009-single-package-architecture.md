# ADR-0009: Single Package Architecture (SUPERSEDED)

**Status:** Superseded by the layered `pkg/` layout introduced in 0.21 and extended in 0.25
**Date:** 2024-12-01
**Deciders:** hyperserve team

## Summary

The original ADR mandated a single-package layout. That decision has been reversed:

- 0.21 (commit `c54c61b`) introduced `pkg/server`, `pkg/websocket`, `pkg/jsonrpc`.
- 0.22 (commit `fb70302`) dropped root facades, requiring `import "github.com/osauer/hyperserve/pkg/server"`.
- 0.25 introduced `pkg/mcp` and `pkg/mcp/builtin`, moving MCP out of `pkg/server` to make the MCP differentiator visible from the import path and to cap `pkg/server` LOC.

The current layout is documented in `ARCHITECTURE.md`. This file remains as a marker so the historical context isn't lost.
