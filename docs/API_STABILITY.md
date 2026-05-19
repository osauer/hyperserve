# API Stability

_Last updated: 2026-05-19 06:35 CEST (v0.34.0)._

## TL;DR

HyperServe is **pre-1.0**. Minor versions (`0.x.0`) may include breaking
changes; patch versions (`0.x.y`) are bug-fix only. Until v1.0.0 lands,
the stability promise is **process, not signatures**: every breaking
change is called out in the [CHANGELOG](../CHANGELOG.md) entry for that
release, with a one-paragraph migration note.

## What pre-1.0 means here

- **No locked signatures.** The package will not pretend a function shape
  is permanent before the surface has stabilised in production use. The
  prior version of this doc tried to lock signatures from v0.9.x; several
  were renamed or restructured by v0.25 (MCP split) and v0.27 (binding
  API), and the lock didn't help anyone.
- **Each minor release is reviewed for subtraction.** v0.25 removed dead
  MCP surface; v0.26 ran a full taste-review sweep and dropped exports
  that had accreted. v0.27 introduced the binding + validation API.
  v0.28–v0.31 layered typed handlers, method-aware route helpers, and
  typed MCP tools on top. v0.32 stabilised the security headers + the
  middleware hot path. v0.33 ran the SSE unexport pass, `Get*` rename,
  and dead MCP metrics removal. **v0.34 is the actually-final breaking
  sweep before v1.0** — `Server.Handle` signature aligned with its
  docstring (`http.HandlerFunc` → `http.Handler`), `MiddlewareRegistry`
  unexported, dead namespace registrars + `ExtensionBuilder.WithConfiguration`
  dropped, plus the security + concurrency fixes called out in
  CHANGELOG. v0.33's release note claimed to be the final sweep; that
  was wrong. v0.34 is the surface v1.0 freezes. Read the release notes
  — the surface moves intentionally.
- **Examples drift with the API.** When a release changes an example's
  shape, the example is updated in the same commit.

## What we do guarantee, even pre-1.0

1. **CHANGELOG accuracy.** Every breaking change appears in `CHANGELOG.md`
   under its release header, with the old and new form side by side
   where helpful.
2. **Compile-fail beats silent change.** When we have to break, we break
   loudly: rename or remove rather than silently change semantics.
3. **`make check` is the floor.** Releases that don't pass
   `gofmt + vet + staticcheck + govulncheck + modernize` do not ship.
4. **Security fixes are patch releases.** Bug fixes that don't change
   shape land as `0.x.(y+1)` and are called out.

## What v1.0.0 will mean

v1.0.0 is **next**, scheduled after v0.34.0 stabilises in production
use. When it lands, this document will lock the public surface, and
these rules will apply:

- **PATCH (`1.0.x`)** — bug fixes, no API changes.
- **MINOR (`1.x.0`)** — additive only. Existing signatures, struct
  fields, and exported types stay backward-compatible.
- **MAJOR (`2.0.0`)** — only if absolutely necessary, with migration
  guide and overlap window.

**No breaking subtractions in 1.x minors.** That is the headline
contract v1.0.0 adds over the current pre-1.0 process: a deletion or
rename of an exported symbol requires v2.0.0, not a minor bump.

Until v1.0.0 lands, treat this library as "stable in shape, mobile in
detail".

## Deprecation policy (pre-1.0)

When something is on its way out:

1. The release notes call it out under "Changed" or "Removed".
2. If it can be aliased (type alias, thin wrapper), it is — see
   `pkg/server.Validate` / `ValidationError` / `FieldError`, kept as
   type aliases when the validation core moved to `internal/validate`
   in v0.31.
3. Hard removal happens on a clean minor boundary, not mid-release.

## Where to look

- **[CHANGELOG.md](../CHANGELOG.md)** — per-release change list and
  migration notes. The contract you can act on today.
- **[ROADMAP.md](./ROADMAP.md)** — what's next; subject to change.
- **GitHub Issues** — `https://github.com/osauer/hyperserve/issues` for
  concrete bug reports.
- **GitHub Discussions** —
  `https://github.com/osauer/hyperserve/discussions` for "is this the
  right shape?" before a release locks the answer.

## If something breaks

1. Check the CHANGELOG entry for the release you upgraded into.
2. If the break isn't documented, file an issue — that's a bug in this
   process, not just in the code.
3. If you're depending on a pre-1.0 release in production, pin to a
   specific version in `go.mod` until v1.0.0 lands.
