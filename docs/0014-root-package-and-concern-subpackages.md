# ADR-0014: Branded root package and concern-specific subpackages

**Status:** Accepted
**Date:** 2026-08-27
**Deciders:** HyperServe maintainer
**Supersedes:** [ADR-0009](./0009-single-package-architecture.md)

## Context

The intermediate public layout placed the central HTTP type in `pkg/server` and
required callers to distinguish the repository name, the `server` package, a
configured `srv` variable, and `*server.Server`. Concern packages also lived
under `pkg/`, even though Go module roots are already a package boundary.

That layout made the introductory import less direct and left rate-limit policy
inside `Server`, where constructor defaults, client identity, quota sharing,
storage bounds, and cleanup lifecycle were coupled to HTTP serving.

The v2 module had few known consumers and the cost of correcting the public
shape was still containable. The alternative was to preserve the intermediate
layout indefinitely or create `/v3` solely for package naming. The accepted
direction is a controlled, explicitly disclosed reset inside `/v2`.

## Decision

Keep the module declaration:

```text
module github.com/osauer/hyperserve/v2
```

The canonical public packages are:

```text
github.com/osauer/hyperserve/v2
github.com/osauer/hyperserve/v2/auth
github.com/osauer/hyperserve/v2/jsonrpc
github.com/osauer/hyperserve/v2/mcp
github.com/osauer/hyperserve/v2/mcp/builtin
github.com/osauer/hyperserve/v2/ratelimit
github.com/osauer/hyperserve/v2/websocket
```

The root package is named `hyperserve`. Its central type is `Server`, and its
constructor is `New(options ...Option) (*Server, error)`.

Old `pkg/...` packages, forwarding facades, and a `NewServer` alias are not
retained. Migration is explicit rather than hidden behind two public shapes.

Rate limiting moves to `ratelimit.New(Config)`, which returns standard HTTP
middleware. The application decides whether and where to attach the gate. The
root server owns no limiter state or cleanup lifecycle.

## Dependency direction

The public graph remains cycle-free:

```text
hyperserve root -> mcp -> jsonrpc
hyperserve root -> websocket
mcp/builtin -> hyperserve root + mcp
ratelimit -> standard library + golang.org/x/time/rate
```

The root package does not import `mcp/builtin` or `ratelimit`.
`mcp/builtin` registers optional hooks during its own initialization, so an
application enabling builtins must explicitly import it.

## Compatibility disposition

This decision ships in v2.1.0 on 2026-08-27. It is a narrow, acknowledged
breaking change inside the existing `/v2` path and therefore outside ordinary
semantic-version compatibility. The migration guide and rollback pin to
v2.0.3 must precede upgrade commands in release-facing documentation.

This exception does not become policy. After v2.1.0, future breaking exported
API changes require a new major module path.

## Consequences

Benefits:

- the introductory import and constructor carry the HyperServe name directly;
- concern packages have short, canonical paths;
- rate-limit policy and quota ownership are explicit at the call site;
- package dependencies state the architectural boundaries rather than a
  generic `pkg/` directory.

Costs:

- v2.0.x source does not compile until imports and the constructor are migrated;
- applications using server-owned limiter options must make policy explicit;
- documentation, examples, scaffold output, and consumers must move atomically
  because no compatibility facade exists.

## Verification

Release acceptance requires package-graph checks, stale-import scans, root and
subpackage tests, race tests, generated-project tests, a downstream consumer
witness, exact-SHA CI, and fresh public-module retrieval.
