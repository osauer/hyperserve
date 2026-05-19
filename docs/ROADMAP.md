# HyperServe Roadmap

_Last updated: 2026-05-19 06:28 CEST (post v0.33.1)._

This document is the project's north star — what HyperServe is, what
differentiates it, and what's planned next. Concrete near-term work is
tracked in [GitHub Issues](https://github.com/osauer/hyperserve/issues);
broader "is this the right shape?" discussion lives in
[GitHub Discussions](https://github.com/osauer/hyperserve/discussions).

## Pitch

A Go HTTP framework with built-in MCP. `net/http` plus one transitive dependency
(`golang.org/x/time`), and an in-tree MCP server so AI assistants can introspect
and operate the same binary that serves traffic.

## Differentiation

- **MCP in-process.** Tools, resources, namespaces, discovery — no out-of-process bridge or third-party SDK.
- **Two-line `go.sum`.** For teams where supply-chain review is a real meeting, this is the headline. Gin's transitive tree is sizable; HyperServe's is one package.
- **Standard-library WebSocket + JSON-RPC + os.Root static serving.** None of these are individually unusual; the combination without dependencies is.

## Near-Term Roadmap (High-Impact, Moderate Effort)

| Theme | Description | Impact | Effort Notes |
|-------|-------------|--------|--------------|
| **1. OpenTelemetry Export Bridge** | Provide `WithOTLPExporter` options for metrics/traces, using the OTLP HTTP protocol and exposing summaries back through an MCP observability tool. | Unlocks integration with Grafana, Datadog, New Relic while reinforcing the AI-observability narrative. | Implement HTTP exporter (no full SDK) and reuse existing metrics registry; add MCP endpoints for curated queries. |
| **2. Runtime Control Safeguards** | Introduce a privileged MCP namespace for safe toggles: reload config, rotate log level, drain WebSocket pools, update rate limits. Ship with RBAC hooks and guardrails. | Makes the “AI-augmented DevOps” story tangible, enabling runbook automation through MCP while keeping SOC teams comfortable. | Wrap existing configuration knobs; add policy hooks and structured auditing. |
| **3. v1.0 freeze** | One more breaking sweep in v0.33 (cohesion split of `pkg/server/server.go`, unexport unused public surface, drop `Get*` prefixes, close the discovery substring leak), then cut v1.0 with `API_STABILITY.md` enforced — no further breaking subtractions in minors. | Closes the single biggest real gap downstream consumers feel today: "every minor is a refactor day." | Subtractions are cheap; the work is mostly choosing what stays and writing the migration notes. |

These items deepen HyperServe’s differentiation (AI-native + secure + production-ready) without compromising the lightweight core.

## Next Build Focus

1. **Ship v0.33 breaking sweep** – Cohesion-split `pkg/server/server.go`, unexport the SSE state machine, drop the remaining `Get*` prefixes, fix the discovery substring leak, raise `pkg/mcp` coverage from 33% → 60%.
2. **Cut v1.0 with `API_STABILITY.md` teeth** – No further breaking subtractions in minor releases. Patch-only signature changes after v1.0.0.
3. **Kick off OTLP bridge** – Sketch the metrics/trace exporter API, flesh out configuration knobs, and capture benchmark baselines before adding collectors.
4. **Prototype runtime controls** – Define the privileged MCP namespace, enumerate the safe toggles, and wire auditing stubs so RBAC can be layered in next.

## One-Click Bundles (Exploration)

- **Goal**: Deliver pre-built HyperServe applications that end users can deploy with a single command.
- **Approach**: Create a `hyperserve bundle` workflow that vendors the backend/frontend, emits Docker/Compose assets, and publishes signed artifacts alongside scaffold templates.
- **Separation of personas**: Keep `hyperserve-init` focused on developers, while bundles target operators or end users who want a turnkey deploy.
- **Open questions**: Distribution channel (GitHub releases vs container registry), update cadence, and how to surface bundle links prominently in the README/downloads.
