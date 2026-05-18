# Security Policy

## Supported versions

HyperServe is pre-1.0 (current: v0.27.x). Only the most recent minor
version receives security fixes. The API may change between minor versions
per [docs/API_STABILITY.md](docs/API_STABILITY.md) — pin to a tagged
release for production.

## Reporting a vulnerability

Email **oliver.sauer@gmail.com** with:

- Affected version (`hyperserve -version` if you built the CLI, otherwise
  the tag or commit you imported)
- Reproduction steps or proof-of-concept
- The class of issue (auth bypass, SSRF, RCE, credential exposure, etc.)

Please do **not** open a public GitHub Issue for security reports.

## Disclosure timeline

- Acknowledgment within **7 days**.
- Fix or mitigation within **90 days**, faster for actively-exploited
  issues.
- Coordinated disclosure: a GitHub Security Advisory (and CVE where
  applicable) goes out alongside the fix release.

## What in scope looks like

The v0.27.0 release notes are a good reference point: seven concrete
vulnerability classes in the built-in MCP surface (SSRF, credential capture
via stored request headers, cross-client SSE injection, path traversal in
file tools, missing tool-call timeout, …) — found, fixed, and disclosed
together. Reports in the same vein are exactly what this policy is for.

Out of scope:

- Issues that require write access to the operator's machine to exploit.
- Theoretical concerns without a reproduction.
- Findings against forks or stale tags older than the current minor.
