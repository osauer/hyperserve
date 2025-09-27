# HyperServe Product Vision

## Elevator Pitch

HyperServe is the Go server that lets humans and AI assistants co-manage the same production workload. It combines a zero-dependency, high-performance HTTP core with native Model Context Protocol (MCP) control surfaces so operations teams can automate safely without surrendering the Go standard library ergonomics they expect.

## Unique Value Proposition

HyperServe is the Go server for AI-augmented operations. It gives teams:

- **AI-native control plane** – Model Context Protocol is built-in, so assistants can discover, introspect, and operate the server without custom glue code.
- **Security-first defaults** – Hardened headers, os.Root sandboxing, optional FIPS/TLS-ECH, and zero third‑party dependencies keep regulated environments comfortable.
- **Operational ergonomics** – Health/readiness servers, graceful shutdown, templating, and connection pooling are available out of the box, making the same binary viable from laptop to production.

Together these priorities let teams ship services that humans and AI co-manage with minimal surface area and minimal supply-chain risk.

## Near-Term Roadmap (High-Impact, Moderate Effort)

| Theme | Description | Impact | Effort Notes |
|-------|-------------|--------|--------------|
| **1. OpenTelemetry Export Bridge** | Provide `WithOTLPExporter` options for metrics/traces, using the OTLP HTTP protocol and exposing summaries back through an MCP observability tool. | Unlocks integration with Grafana, Datadog, New Relic while reinforcing the AI-observability narrative. | Implement HTTP exporter (no full SDK) and reuse existing metrics registry; add MCP endpoints for curated queries. |
| **2. Runtime Control Safeguards** | Introduce a privileged MCP namespace for safe toggles: reload config, rotate log level, drain WebSocket pools, update rate limits. Ship with RBAC hooks and guardrails. | Makes the “AI-augmented DevOps” story tangible, enabling runbook automation through MCP while keeping SOC teams comfortable. | Wrap existing configuration knobs; add policy hooks and structured auditing. |
| **3. Project Bootstrap & Templates** | Deliver a `hyperserve init` CLI that scaffolds secure-by-default services (config, Dockerfile, example MCP tools, OTLP wiring). | Reduces time-to-first-value for new teams, demonstrates best practices, and increases perceived polish. | Build on current examples; generate code via text/template with minimal dependencies. |

These items deepen HyperServe’s differentiation (AI-native + secure + production-ready) without compromising the lightweight core.

## Next Build Focus

1. **Kick off OTLP bridge** – Sketch the metrics/trace exporter API, flesh out configuration knobs, and capture benchmark baselines before adding collectors.
2. **Prototype runtime controls** – Define the privileged MCP namespace, enumerate the safe toggles, and wire auditing stubs so RBAC can be layered in next.
3. **Expand `hyperserve-init` templates** – Layer in OTLP-ready wiring, runtime control stubs, and template variants so the scaffold stays aligned with the broader roadmap.

## One-Click Bundles (Exploration)

- **Goal**: Deliver pre-built HyperServe applications (starting with the Regime project in `../regime`) that end users can deploy with a single command.
- **Approach**: Create a `hyperserve bundle` workflow that vendors the backend/frontend, emits Docker/Compose assets, and publishes signed artifacts alongside scaffold templates.
- **Separation of personas**: Keep `hyperserve-init` focused on developers, while bundles target operators or end users who want a turnkey deploy.
- **Open questions**: Distribution channel (GitHub releases vs container registry), update cadence, and how to surface bundle links prominently in the README/downloads.
