# Security Policy

## Supported Versions

HyperServe is a v1 Go module. The latest v1 release receives security fixes;
older tags are supported only when a report clearly affects a maintained
downstream deployment. Breaking API changes require a future major module path.

## Reporting A Vulnerability

Email **oliver.sauer@gmail.com** with:

- Affected tag or commit
- Reproduction steps or proof-of-concept
- Issue class, such as auth bypass, SSRF, RCE, credential exposure, path traversal, or cross-client injection

Please do **not** open a public GitHub Issue for security reports.

## Disclosure Timeline

- Acknowledgment within **7 days**.
- Fix or mitigation within **90 days**, faster for actively exploited issues.
- Coordinated disclosure through a GitHub Security Advisory and CVE where applicable.

## Scope

Reports against HyperServe's MCP, SSE, JSON-RPC, WebSocket, static-file, auth,
configuration, and scaffold surfaces are in scope. Findings that require write
access to the operator's machine, lack a concrete reproduction, or target forks
and stale unsupported tags are out of scope.
