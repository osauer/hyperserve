# One-Click Bundle Exploration

## Objective

Provide an installable bundle that delivers the Regime application backed by HyperServe in a single command, targeting end-users who want the experience without building from source.

## Constraints & Assumptions

- Base application lives in `../regime` and already uses HyperServe for its backend.
- Bundle should be reproducible, versioned, and verifiable (signed checksums or SBOM).
- Distribution channels must work without requiring Go toolchains (e.g. container images, prebuilt binaries, or packaged archives).
- Keep `hyperserve-init` focused on developers; the bundle is an operator/end-user artifact.

## Candidate Delivery Mechanisms

1. **Container Image**
   - Build multi-stage Dockerfile that compiles the Regime backend, bundles static frontend assets, and ships a hardened distroless runtime.
   - Publish to GHCR (`ghcr.io/osauer/regime-bundle:<version>`).
   - Pros: one `docker run` away, easy for Kubernetes.
   - Cons: Requires Docker/Podman; less friendly for bare-metal installs.

2. **Docker Compose Stack**
   - Package backend + frontend + PostgreSQL using Compose.
   - Provide `curl | sh` installer that pulls compose YAML and env template.
   - Pros: Smooth local dev/test, matches existing Regime repo setup.
   - Cons: Heavier footprint; relies on Docker even for users who only need backend + SQLite.

3. **Self-contained Tarball**
   - `hyperserve bundle regime --output regime.tar.gz` emits binaries, configs, systemd unit, and migration scripts.
   - Pros: Works on air-gapped servers, easy to verify signatures.
   - Cons: Requires manual install steps (unpack, configure, start service).

A hybrid approach could publish both a container image and an artifact tarball built from the same pipeline.

## Proposed Command

```
hyperserve bundle regime \
  --source ../regime \
  --tag v0.1.0 \
  --image ghcr.io/osauer/regime-bundle:v0.1.0 \
  --artifact dist/regime-v0.1.0.tgz
```

### Steps

1. Validate source tree (ensure expected directories, go.mod, frontend build scripts).
2. Run `npm install && npm run build` inside the frontend to produce static assets.
3. Execute `go build` for the backend with `CGO_ENABLED=0` and embed static assets if needed.
4. Emit Dockerfile + Compose templates that reference the built artifacts.
5. Package artifacts + template configs into the requested archive.
6. Optionally `docker build` and push the image when `--image` is provided.

## Pipeline Considerations

- Use `mage` targets or Go build tags to strip debug info.
- Generate SBOM via `syft` or `govulncheck` integration, embed in release.
- Publish GitHub Actions workflow leveraging matrix builds (linux/amd64, linux/arm64).
- Reuse HyperServe's `Makefile` patterns where possible to avoid drift.

## Open Questions

1. How do we expose MCP endpoints safely in the bundle by default? (Likely disabled with opt-in flag.)
2. Should database migrations run automatically on start or via a preflight command?
3. Do we ship `hyperserve-init` generated config as a starting point for customization?
4. Where do we surface bundle downloads? (README hero section, docs/SCAFFOLDING.md, release notes.)

## Next Actions

- Audit the Regime repo to catalog build inputs and runtime requirements.
- Prototype `hyperserve bundle` command with dry-run output (generate plan without building).
- Draft documentation section under `docs/BUNDLE_EXPLORATION.md` into a future "Bundles" guide once implementation exists.
