# Repository guide

[Architecture](ARCHITECTURE.md) owns the package graph and responsibility map.
[API stability](docs/API_STABILITY.md) owns supported import surfaces.

| Task | Start here |
|---|---|
| Run a first server | [README](README.md), then [focused examples](examples/README.md) |
| Understand transport and lifecycle ownership | [Architecture](ARCHITECTURE.md) |
| Deploy an application | [Production](docs/PRODUCTION.md) |
| Generate an application | [Scaffolding](docs/SCAFFOLDING.md) |
| Change the library | [Contributing](CONTRIBUTING.md) |
| Migrate a consumer | [module migration](docs/MIGRATING.md) |
| Understand past decisions | [Decision history](docs/DECISIONS.md) |

`Makefile` owns the executable checks. `internal/archcheck` verifies public
package boundaries; `internal/doccheck` verifies current documentation links
and known retired APIs. `tools/` isolates development-only dependencies.
