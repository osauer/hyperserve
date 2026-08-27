# Security Policy

## Supported versions

The latest release on the `github.com/osauer/hyperserve/v2` module line
receives security fixes. Older v2 tags and the v1 module are not generally
maintained, though reports that clearly affect a maintained downstream
deployment are still useful.

The v2.1.0 package reset is documented in
[Migrating to v2.1](docs/MIGRATING_V2_1.md). It does not weaken the normal
policy: future breaking public API changes require a new major module path.

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
