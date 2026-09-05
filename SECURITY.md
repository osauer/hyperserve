# Security Policy

## Supported versions

Only the latest stable HyperServe release receives bug fixes and security
updates. Fixes ship in the next release; older releases receive no backports.
Older tags remain available for reproducible builds.

See [API stability](docs/API_STABILITY.md) for versioning and migration guidance.

## Reporting a vulnerability

Email **oliver.sauer@gmail.com** with:

- the affected tag or commit;
- reproduction steps or a proof of concept;
- the issue class, such as authentication bypass, SSRF, remote code execution,
  credential exposure, path traversal, denial of service, or cross-client
  injection.

Do not open a public GitHub issue for a security report.

## Disclosure timeline

- Acknowledgment within 7 days.
- Fix or mitigation within 90 days, faster for active exploitation.
- Coordinated disclosure through a GitHub Security Advisory and CVE when
  applicable.

## Scope

The root HTTP package, MCP, SSE, JSON-RPC, WebSocket, static-file, authentication,
rate-limit, configuration, and scaffold surfaces are in scope.

The root server does not own application authorization or trusted-proxy policy.
That boundary does not make bypasses in HyperServe's authentication middleware,
proxy-chain parser, protocol validation, or default-deny behavior out of scope.

Findings that require write access to the operator's machine, lack a concrete
reproduction, or affect only forks and unsupported tags are normally out of
scope.
