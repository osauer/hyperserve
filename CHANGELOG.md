# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.26.0] - 2026-05-18

Taste-review sweep. Targets dead surface, half-wired features, and doc drift
that survived the 0.25.x cleanups. Net change: **−2,479 LOC** across 38 files
plus a removed 9.6 MB tracked binary. Breaking — see Migration.

### Added
- `pkg/server/options_mcp.go` — five MCP option constructors (`WithMCPSupport`,
  `WithMCPEndpoint`, `WithMCPFileToolRoot`, `WithMCPBuiltinTools`,
  `WithMCPBuiltinResources`) moved out of `server.go` so the MCP glue lives in
  one place.
- Warn-log when `WithMCPBuiltinTools(true)` / `WithMCPBuiltinResources(true)`
  is set but the `pkg/mcp/builtin` blank-import is missing, with the exact
  import line to fix it. Closes the silent half-wire that previously made
  `cmd/server -mcp` advertise tools and register none.
- `cmd/server/main.go` blank-imports `pkg/mcp/builtin` so the bundled binary
  actually serves the built-in tools/resources it advertises.

### Changed
- Minimum Go version bumped to **1.26** (was 1.25). `go.mod`, CI workflows,
  and `internal/scaffold/templates/go.mod.tmpl` all aligned. See
  [ADR-0006](docs/0006-go-minimum-version.md).
- `pkg/mcp.Handler` now builds `tools/list`, `resources/list`,
  `resources/read`, and `tools/call` responses from the exported `ToolInfo`,
  `ResourceInfo`, `ResourceContent`, `ToolResult` types instead of
  `map[string]any` literals. Wire shape is unchanged; in-process callers
  using `ProcessRequestDirect` see typed structs instead of maps.
- `ToolResult` gained an `IsError bool` field (`omitempty`) so error
  responses round-trip through the type.
- `MiddlewareRegistry.applyToMux` sorts route keys deterministically
  (ascending length, ties alphabetical) before chaining; map-iteration
  randomness no longer leaks into middleware order when multiple prefixes
  match a request.
- `applyEnvVars` is now table-driven (`defaultEnvBindings` + `parseEnvBool`
  helpers); ~70 LOC shorter and a new env var is one entry, not nine
  branches.
- `NewServer` split into five named helpers (`newServerSkeleton`,
  `applyConfiguredLogLevel`, `autoConfigureMCPFromEnv`, `openTemplateRoot`,
  `initializeMCPHandler`); the top-level body is now ~30 LOC of
  orchestration.
- `rateLimiterEntry.lastAccess` is now `lastAccessUnixNano atomic.Int64`;
  hot-path requests bump the timestamp without re-acquiring the
  rate-limiter pool's write lock.
- `Server.websocketConnections` renamed to `totalWebSocketUpgrades`; the
  metrics-log key changed from `websocket-connections` to
  `websocket-upgrades-total` to reflect that it counts lifetime upgrades,
  not concurrent sessions.
- `WithFIPSMode` doc-comment scoped honestly: "FIPS-approved TLS cipher
  suites + curves" (not "FIPS 140-3 compliance"). The runtime log line
  matches.

### Removed (breaking)
- `pkg/server/interceptor.go` and its tests (`InterceptorChain`,
  `Interceptor`, `InterceptableRequest`, `InterceptableResponse`,
  `InterceptorResponse`, `AuthTokenInjector`, `RateLimitInterceptor`,
  `RequestLogger`, `ResponseTransformer`). Parallel pipeline beside
  `MiddlewareRegistry` with zero non-example callers; `pkg/server/middleware.go`
  provides equivalent middleware. `examples/interceptors/` removed.
- `pkg/websocket/websocket_pool.go` and its test (`WebSocketPool`,
  `pooledConn`). `Get(ctx, endpoint, upgrader, w, r)` returned a connection
  bound to a previous request's socket without ever upgrading the current
  request — broken-by-design for server-side WebSocket. `examples/websocket-pool/`
  removed.
- `WithOutStack` server method + `MiddlewareRegistry.exclude` field +
  `filterMiddleware` function. Exported orphan method (zero callers) that
  dragged a reflect-based comparison loop along.
- `WithLogger` server option. Mutated a package-global without a mutex.
  Use `pkg/server.SetDefaultLogger(l)` (or `pkg/mcp.SetDefaultLogger`)
  instead.
- `WithMCPNamespace` server option (zero callers). Use
  `srv.RegisterMCPNamespace(name, configs...)` post-construction.
- `ResponseTimeMiddleware` (existed only as the prohibited pattern in one
  log-test assertion).
- `FileServer` middleware-stack constructor — verbatim copy of `SecureWeb`.
  Name also collided with `http.FileServer`.
- `Options.DeferredInit` field (`json:"-"`) and its NewServer
  reconciliation. `WithDeferredInit` now writes only to
  `srv.deferred.init`.
- `pkg/mcp.LoggingCapability`, `PromptsCapability`, `SamplingCapability` —
  empty structs never instantiated. The MCP `Capabilities` struct keeps
  only `Resources`, `Tools`, `SSE`, `Experimental`.
- Three `t.Skip("requires full connection")` stub tests in
  `pkg/server/websocket_handlers_test.go`.
- `cmd/example-server/` — 75-line near-duplicate of `cmd/server` with no
  caller; the PROJECT_STRUCTURE.md claim that benchmarks used it was
  outdated.
- `spec/conformance/` — orphan `package main` not invoked by any Makefile
  target, CI job, or doc command.
- `docs/MIGRATION_GUIDE.md` (stale 1.24 migration content), 
  `docs/RELEASE_NOTES.md` (duplicate of CHANGELOG with `interface{}` API
  examples), `docs/PUBLISH_CHECKLIST.md` (one-shot 2025-06-27 launch
  checklist).
- 9.6 MB tracked `deferred-init` Mach-O binary at the repo root, untracked
  and added to `.gitignore`.

### Fixed
- `WithTimeouts(...)` no longer panics at `NewServer` time. `setTimeouts`
  previously wrote `srv.httpServer.X` while `httpServer` was still nil
  (it's built lazily in `StartServer`); now only `srv.Options.X` is set,
  which `StartServer` reads when constructing `http.Server`.
- `cmd/server -mcp` previously advertised built-in tools without
  registering any (the `pkg/mcp/builtin` blank-import was missing and the
  hook-nil check was silent). Fixed by adding the blank import and
  surfacing the misconfiguration as a warn-log.
- README headline claim "`go.sum` has two lines" reworded to reflect the
  actual count (`go.sum` is 12 lines after the `tool` directive pulled in
  the modernize-check transitive deps); ARCHITECTURE.md follows suit.
- `examples/README.md` enumerates every actual example directory; the
  stale `examples/mcp/` reference is gone, the AI-tell star-rating ladder
  is gone, and the v0.25.0 additions (`deferred-init`, `devops`) are now
  listed.
- `pkg/mcp/builtin/server_tools.go` `server_control.restart`,
  `server_control.reload`, and `request_debugger.replay` actions removed
  from the JSON schema enums, descriptions, and dev_guide. They returned
  canned success strings without performing the advertised action —
  direct violation of the project's DoD ("no mocks, stubs, or simulated
  behaviour unless explicitly asked").
- PROJECT_STRUCTURE.md, ARCHITECTURE.md, CLAUDE.md, README.md no longer
  carry stale parenthetical `(Go 1.24)` / `(Go 1.24+)` feature
  attributions; the minimum lives in `go.mod`.
- `benchmarks/run_benchmarks.sh` now targets `./cmd/server` (was the
  stale `./cmd/hyperserve` path).

### Migration

1. **`WithLogger(l)` → `SetDefaultLogger(l)`.** The functional option
   mutated a package global; the explicit setter is now the only path.
2. **`WithMCPNamespace(name, configs...)` → `srv.RegisterMCPNamespace(name, configs...)`** after `NewServer`.
3. **`WithOutStack` callers**: pass a smaller stack to `WithMiddleware`
   instead of subtracting; there were no known callers.
4. **`pkg/server.FileServer(opts)` callers**: replace with
   `pkg/server.SecureWeb(opts)` (the bodies were already identical).
5. **`ResponseTimeMiddleware` callers**: register
   `RequestLoggerMiddleware` and read the duration from its log line,
   or write a one-line custom timing middleware.
6. **`pkg/server/interceptor.go` consumers**: rewrite on
   `pkg/server/middleware.go` (`AuthMiddleware`,
   `RequestLoggerMiddleware`, `RateLimitMiddleware`).
7. **`pkg/websocket.WebSocketPool` consumers**: there were none in the
   examples tree because the type was broken-by-design for server-side
   upgrades.
8. **`pkg/mcp.LoggingCapability` / `PromptsCapability` /
   `SamplingCapability`**: these were never instantiated; references will
   not compile, but no working code path could have used them.
9. **`pkg/mcp.Handler.ProcessRequestDirect` callers** type-asserting to
   `map[string]any`: switch to `mcp.ToolResult`, `mcp.ToolInfo`,
   `mcp.ResourceInfo`, `mcp.ResourceContent` for `tools/call`,
   `tools/list`, `resources/list`, and `resources/read` payloads
   respectively.

## [0.25.1] - 2026-05-17

Patch release. Drives `staticcheck ./...` to zero across pre-existing findings the
0.25.0 `make check` gate exposed, and wires that gate into CI so it runs on every
push and PR. No API changes.

### Changed
- CI (`.github/workflows/ci.yml`) bumped to Go 1.25 (matches `go.mod`), now runs `make test` (which runs the full `check` gate then `go test`), installs `staticcheck` and `govulncheck` explicitly, and builds `./cmd/server` (was the stale `./cmd/hyperserve` path).
- `Makefile`: `test` target now depends on `check`, so local `make test` runs the gofmt + vet + staticcheck + govulncheck + modernize gate before the test suite.
- Error strings in `pkg/server/server.go` lowercased / unpunctuated per Go style (ST1005).

### Removed
- `internal/responsewriter` package — dead since the websocket package landed; zero callers.
- `pkg/server.trailingSlashMiddleware` — unreferenced and the inline TODO already noted it was likely obsolete.
- `pkg/server.(*Server).shutdownHealthServer` — unused (health server shutdown happens inline).
- `pkg/websocket.isWebSocketUpgrade` — unused; the `Upgrade` method does its own checking inline.
- Unused struct fields: `InterceptableResponse.{written,mu}`, `lowConn.closeErr`, `Conn.{pingInterval,pongTimeout}`.
- `spec/conformance.fetchResponse` and its `io` import — unused since the conformance suite landed.
- `.golangci.yml` plus its CI job — `make check` covers the same ground via tools pinned in `go.mod` (vet + staticcheck + modernize) plus govulncheck.

### Fixed
- `pkg/server/server.go` no longer registers `syscall.SIGKILL` with `signal.Notify` — it cannot be trapped (SA1016).
- Health-server `BaseContext` no longer attaches a string-keyed value that nothing reads (SA1029).
- Misc S1023 / S1039 cleanups (redundant return, unnecessary `fmt.Sprintf`) in `pkg/server` and `pkg/websocket`.

## [0.25.0] - 2026-05-17

Pre-1.0 cleanup release. This is a breaking-API change: import paths and several
exported names have moved. See the migration notes below.

### Added
- New package `pkg/mcp` — the MCP protocol surface (Handler, transports, discovery, namespaces, metrics, builders) now lives in its own package, depending only on `net/http`, `log/slog`, and `pkg/jsonrpc`.
- New package `pkg/mcp/builtin` — opt-in built-in MCP tools (Calculator, FileRead, HTTPRequest, ListDirectory) and resources (Config, Metrics, System, ServerLog, ServerHealth) plus the dev-mode toolkit (ServerControl, RouteInspector, RequestDebugger, DevGuide). Blank-import this package to wire `WithMCPBuiltinTools/Resources(true)` and the `MCPDev()` / `MCPObservability()` presets.
- `examples/deferred-init/` — end-to-end demo of `WithDeferredInit` + `WithOnReady` + `WithDeferredInitStopOnFailure`.
- `make check` — runs `vet`, `staticcheck`, `modernize`, `govulncheck`; installs each lazily on first use with pinned versions.
- Several test/observation accessors on `*server.Server`: `MCPHandler()`, `IsRunning()`, `IsReady()`, `ServerStart()`, `TotalRequests()`, `TotalResponseTime()`, `ClientLimiterCount()`, `MiddlewareRoutes()`, `Mux()`, `SetMetrics()`, `AddMetrics()` — required by tests and the builtin package now that they live outside `pkg/server`.

### Changed
- Minimum Go version bumped to **1.25** (was 1.24). Unlocks `sync.WaitGroup.Go(...)`, `testing/synctest`, and container-aware `GOMAXPROCS`. See [ADR-0006](docs/0006-go-minimum-version.md).
- `Server` struct shrunk from 33 fields to 22; deferred-init lifecycle fields extracted into `deferredLifecycle` sub-struct; rate-limiter pool fields extracted into `rateLimiterPool` sub-struct.
- `pkg/websocket` collapsed `internal/ws` back into the public package — the internal/external split was unnecessary indirection. (Subagent task.)
- README, ARCHITECTURE.md, PROJECT_STRUCTURE.md, SCAFFOLDING.md, PRODUCT_VISION.md, MIGRATION_GUIDE.md, RELEASE_NOTES.md, LESSONS_LEARNED.md, and ADR-0006/0009 rewritten to remove AI-generated stylistic tells (adjective stacks, emoji ladders, hollow superlatives) and to reflect the new package layout.

### Removed
- `WithEncryptedClientHello`, `Options.EnableECH`, `Options.ECHKeys` — the option captured keys but never installed them on `tls.Config.EncryptedClientHelloKeys`. Advertised security feature that did nothing. (Reintroduce with a real handshake test when needed.)
- `ChaosMode` / `ChaosMaxLatency` / `ChaosMinLatency` / `ChaosErrorRate` / `ChaosThrottleRate` / `ChaosPanicRate` fields, `ChaosMiddleware`, and `examples/chaos/` — no `WithChaos*` setter ever existed, so the feature was unreachable via the supported API.
- Deprecated MCP options: `WithMCPToolsDisabled()`, `WithMCPResourcesDisabled()`, `WithMCPServerInfo()` — zero callers; superseded by `WithMCPBuiltinTools(bool)`, `WithMCPBuiltinResources(bool)`, and the `name`/`version` args of `WithMCPSupport`.
- `LogResource` + `NewLogResource` + `AddLogEntry` — dead twin of `ServerLogResource` that was the version registered in MCP standard mode. The standard-mode log resource (`logs://server/recent`) now returns the working `ServerLogResource` payload.
- `pkg/server/jsonrpc_facade.go` — internal alias file with no external importers. All MCP code in `pkg/mcp` now imports `pkg/jsonrpc` directly.
- `pkg/server/websocket_facade.go` — internal alias file with one external example caller that already imported `pkg/websocket` directly. (Subagent task.)
- `internal/ws/` directory — merged into `pkg/websocket`. (Subagent task.)
- 10 MB tracked Mach-O binary at the repo root.

### Fixed
- Standard-mode MCP `logs://server/recent` resource is now wired to the working log buffer; previously always returned empty.
- Pre-existing race in `TestMiddlewareWithWebSocket` (handler-call atomic was asserted before the server goroutine could store it) — added a short bounded wait.

### Migration

1. **Imports.** Any code referencing `server.MCPHandler`, `server.MCPTool`, `server.MCPResource`, `server.NewMCPExtension`, `server.NewTool`, `server.NewResource`, etc. should import `mcp "github.com/osauer/hyperserve/pkg/mcp"` and use `mcp.Handler`, `mcp.Tool`, `mcp.Resource`, `mcp.NewExtension`, `mcp.NewTool`, `mcp.NewResource`. The full rename table is in the CHANGELOG history of `pkg/mcp/types.go` and in the migration guide.
2. **Built-in MCP tools.** Add `_ "github.com/osauer/hyperserve/pkg/mcp/builtin"` to your `main` package so the `WithMCPBuiltinTools(true)` / `MCPDev()` / `MCPObservability()` hooks fire. Without the blank import, the option calls are no-ops.
3. **Extension API.** `mcp.Extension.Configure` now takes `*mcp.Handler` instead of `*server.Server`. If your extension needs `*Server` access, refactor it to read the handler off the server (`srv.MCPHandler()`) at registration time.
4. **Removed surface.** Replace `WithMCPServerInfo(name, version)` with `WithMCPSupport(name, version)`; drop calls to `WithMCPToolsDisabled()` / `WithMCPResourcesDisabled()` (defaults are already off); remove any `WithEncryptedClientHello(...)` calls and any reliance on `Options.EnableECH/ECHKeys`; drop `ChaosMode` references.

## [0.24.0] - 2025-10-19

### Added
- `CompleteDeferredInit` helper for manually finalizing deferred bootstrap sequences without restarting the process.

### Fixed
- Ensured deferred-init readiness gating is respected by the running HTTP server and avoids matching health endpoint prefixes.
- Allowed OnReady hooks to rerun after failures and improved manual recovery paths for non-fatal bootstrap errors.

## [0.23.0] - 2025-10-19

### Added
- Deferred initialization lifecycle support via `WithDeferredInit`, readiness gating, and `WithOnReady` hooks for post-bootstrap route registration.
- Startup banner color control with `WithBannerColor` and the `HS_BANNER_COLOR` environment flag.

## [0.22.0] - 2025-09-29

### Changed
- Removed root compatibility facades; consumers now import `github.com/osauer/hyperserve/pkg/server` directly.
- Updated all binaries, examples, and documentation to the new package layout.
- Simplified Makefile targets to build via the `cmd/server` entry point.

## [0.21.0] - 2025-09-29

### Changed
- Moved server, WebSocket, and JSON-RPC implementations into versioned `pkg/` packages with compatibility facades.
- Simplified top-level repository layout and aligned architecture documentation.
- Reworked tests to live beside their implementation packages and pruned transient fixture directories.

## [0.20.1] - 2025-09-27

### Added
- Regression coverage for shutdown context propagation and WebSocket pool statistics snapshots.

### Changed
- Brought README, ADRs, guides, and API spec in sync with current behaviour and APIs.
- Hardened MCP SSE and middleware integration tests to use sandbox-friendly listeners or skip gracefully when networking is restricted.
- Documented repository hygiene expectations and broadened ignores for local tooling caches.

### Fixed
- Prevented double-closing of the rate-limiter cleanup ticker during repeated shutdowns.
- `go test ./...` no longer leaves stray `_test.txt` logs and avoids panicking when health-server ports are unavailable.

## [0.19.1] - 2025-07-22

### Added
- **JSON Response for MCP GET Requests** - Enhanced MCP handler for better tool integration (#78)
  - Added robust Accept header parsing with `isJSONAccepted()` function
  - MCP GET endpoint now returns JSON when Accept header contains `application/json` or `*/*`
  - JSON response includes server status, capabilities, endpoint, and transport information
  - Handles quality factors, case sensitivity, and complex Accept headers
  - Added error handling for JSON encoding with proper logging
  - Refactored capabilities into reusable `getCapabilities()` method
  - Added comprehensive test coverage for all Accept header scenarios
  - Improves compatibility with automated tools like Claude Code

## [0.19.0] - 2025-07-20

### Changed
- **Standardized MCP Tool Naming** - All tools now use consistent namespace prefixes
  - Built-in tools now use `mcp__hyperserve__` prefix (e.g., `mcp__hyperserve__calculator`)
  - Developer tools now use `mcp__hyperserve__` prefix (e.g., `mcp__hyperserve__server_control`)
  - External/custom tools maintain their existing namespace pattern (e.g., `mcp__daw__play`)
  - Empty namespace in `RegisterToolInNamespace()` now defaults to server name
  - Updated all tests and documentation to reflect new naming convention
  - This change improves API consistency and prevents naming conflicts

## [0.18.0] - 2025-07-20

### Added
- **Dynamic Discovery with Security Policies** - Enhanced MCP discovery with configurable security
  - Tools/resources can implement `IsDiscoverable()` to opt out of discovery
  - Added `DiscoveryPolicy` enum: Public, Count, Authenticated, None
  - Added `WithMCPDiscoveryPolicy()` and `WithMCPDiscoveryFilter()` server options
  - Discovery endpoints now return dynamic tool/resource lists based on policy
  - Default filtering hides dev tools (server_control, etc) in production
  - Custom filters enable RBAC integration via Authorization headers
  - Created examples/mcp-discovery demonstrating different policies

### Changed
- Discovery endpoints now show actual registered tools/resources instead of static capabilities
- Tool counts and lists respect discovery policies and custom filters

## [0.17.0] - 2025-07-20

### Added
- **MCP Discovery Endpoints** - Implemented Claude Code discovery mechanisms
  - Added `/.well-known/mcp.json` endpoint for standard MCP discovery
  - Added `/mcp/discover` endpoint as alternative discovery mechanism
  - Both endpoints return transport information, capabilities, and connection details
  - Enables automatic MCP server discovery without manual configuration
  - Updated CLAUDE.md and README.md with discovery endpoint documentation

## [0.16.0] - 2025-07-20

### Added
- **Enhanced SSE Documentation** - Improved discoverability of the unified MCP endpoint approach
  - Updated CLAUDE.md with clear SSE connection instructions for AI assistants
  - Added SSE capability to MCP initialize response for automatic discovery
  - Enhanced README.md with SSE support section explaining unified endpoint
  - Expanded MCP_GUIDE.md with detailed SSE documentation and examples
  - Created examples/mcp-sse directory with complete SSE client/server examples

### Changed
- **Simplified MCP Implementation** - Removed backward compatibility for cleaner code
  - `RegisterTool()` and `RegisterResource()` now register without namespace prefixing
  - `RegisterToolInNamespace()` and `RegisterResourceInNamespace()` always apply prefixes
  - Removed complex dual-mode logic, making the API more predictable
  - Net reduction of 17 lines while improving clarity

### Fixed
- **SSE Capability Reporting** - SSE support is now properly advertised in MCP capabilities
  - Added `SSECapability` struct to `MCPCapabilities`
  - Initialize response now includes SSE configuration details
  - Instructions updated to mention SSE support alongside regular HTTP

## [0.15.0] - 2025-07-19

### Added
- **MCP Namespace Support** - Organize tools and resources into logical namespaces
  - New methods: `RegisterMCPToolInNamespace`, `RegisterMCPResourceInNamespace`, `RegisterMCPNamespace`
  - Server option: `WithMCPNamespace` for namespace configuration
  - Tools/resources in namespaces are prefixed with `mcp__namespace__name` format
  - Backward compatibility maintained for non-namespaced registration
  - Default namespace uses server name when not specified
  - Comprehensive test coverage with 8 test scenarios
  - Updated MCP_GUIDE.md with namespace documentation and examples

### Changed
- MCP handler now tracks registered namespaces internally
- Tools and resources use flat maps with prefixed keys for efficient lookup

## [0.14.7] - 2025-07-19

### Fixed
- **MCP Calculator Tool** - Fixed missing calculator tool when builtin tools are enabled
  - Calculator tool is now properly registered with `WithMCPBuiltinTools(true)`
  - Fixes test failures in concurrent and default MCP tests

## [0.14.6] - 2025-07-19

### Fixed
- **Route Inspector Tool** - Now shows all registered routes instead of limiting to 5
  - Fixed iteration logic to properly display all routes in middleware registry
  - Improved route discovery for better debugging capabilities

### Changed
- **Project Structure** - Major cleanup and reorganization for better maintainability
  - Removed duplicate files (docs/CHANGELOG.md, README_NEW.md, outdated READMEs)
  - Consolidated MCP examples from 6 to 4 focused examples:
    - `mcp-basic` (merged mcp + mcp-sse) - Complete basic example with HTTP/SSE
    - `mcp-cli` (merged mcp-flags + mcp-development) - CLI configuration with dev mode
    - `mcp-extensions` - Advanced application integration patterns
    - `mcp-stdio` - Claude Desktop integration
  - Consolidated websocket examples into single `websocket-demo`
  - Cleaned up compiled binaries from examples directory
  - Updated .gitignore to comprehensively exclude all compiled binaries in examples
  - Created `docs/guides/` directory for future guide documents

### Improved
- **Documentation** - Enhanced CLAUDE.md with reference to MCP_GUIDE.md for better AI assistant discovery
- **Repository Hygiene** - Removed temporary test files and maintained single-package architecture for API stability

## [0.14.0] - 2025-07-13

### Added
- **MCP Discovery Improvements** - Enhanced discoverability for AI assistants
  - Prominent MCP discovery banner displayed on server startup in developer mode
  - Shows example curl command for `tools/list` discovery
  - Added `.mcp` marker file for project-level MCP detection
  - Updated CLAUDE.md with immediate action instructions for AI assistants
- **Git Ignore Enhancement**
  - Added `.claude/settings.local.json` to .gitignore to prevent tracking local settings

### Improved
- AI assistants now immediately recognize and utilize MCP capabilities when working with HyperServe projects
- Clear instructions in startup banner for MCP tool discovery

## [0.13.3] - 2025-07-13

### Changed
- **BREAKING**: Removed `SecureWebWithRateLimit` middleware to avoid API bloat
  - Use `SecureWeb` with separate `RateLimitMiddleware` instead
- Implemented "secure by default" approach for server timeouts
  - Default timeouts increased for better compatibility (30s read/write)
  - Automatic Slowloris protection with 10s ReadHeaderTimeout
  - No longer requires explicit configuration for basic security

### Security
- Server now starts with secure timeout defaults out of the box
- Slowloris attacks are mitigated by default with ReadHeaderTimeout

## [0.13.2] - 2025-07-13

### Fixed
- **MCP Request Debugger** - Fixed request capture middleware that was not intercepting HTTP requests
  - Added RequestCaptureMiddleware to actually intercept and store requests
  - Added CaptureRequest method with atomic ID generation
  - Added captureResponseWriter to capture response headers, status, and body
  - Automatic middleware registration in MCP dev mode
  - Memory management with 100 request limit and 64KB response body limit
  - Thread-safe operation using sync.Map

## [0.13.1] - 2025-07-13

### Added
- **Enhanced Security Middleware**
  - New `SecureWebWithRateLimit` middleware stack that combines security headers with optional rate limiting
  - Automatically includes rate limiting only when configured (`RateLimit > 0`)
- **WebSocket Telemetry**
  - WebSocket connections are now tracked in server telemetry
  - New `WebSocketUpgrader()` method on Server that automatically tracks WebSocket metrics
  - WebSocket connection count displayed in server shutdown metrics
  - Helper functions for WebSocket origin validation (`defaultCheckOrigin`, `checkOriginWithAllowedList`)

### Improved
- Enhanced middleware documentation with security examples
- Added comprehensive tests for new security features

## [0.13.0] - 2025-07-13

### Added
- **Enhanced Security Features**
  - Individual timeout configuration options: `WithReadTimeout`, `WithWriteTimeout`, `WithIdleTimeout`, `WithReadHeaderTimeout`
  - Automatic Slowloris attack protection via `ReadHeaderTimeout` (defaults to `ReadTimeout` if not set)
  - Comprehensive security documentation in README
  - Timeout configuration guide with recommendations
  - Integration tests for security features
- **Improved Error Handling**
  - Added `closeWithLog` helper for proper defer close error handling
  - Updated error comparisons to use `errors.Is` and `errors.As`
  - Added error wrapping for better context in external package errors
- **Documentation**
  - Added missing comments on exported types
  - Documented SHA1 usage in WebSocket as RFC 6455 requirement
  - Added security best practices section

### Fixed
- Integer overflow protection in WebSocket frame size handling
- Unchecked errors in defer close statements
- Health server now uses same timeout configuration as main server
- ReadHeaderTimeout properly applied to both main and health servers

### Security
- Mitigated Slowloris attacks with proper timeout configuration
- Protected against integer overflow in WebSocket frame parsing
- Improved error handling to prevent information leakage

## [0.12.2] - 2025-07-13

### Fixed
- MCP tool response formatting now properly handles different return types (strings, maps, arrays)
- Fixed Zod validation errors in Claude by correctly formatting tool responses with content arrays
- Tool responses returning maps/objects are now JSON-serialized to text content

### Added
- Comprehensive test coverage for MCP tool response formatting
- New `dev_guide` tool for better MCP developer experience
  - Interactive help system showing available tools and resources
  - Usage examples and common workflows
  - Topic-based documentation (overview, tools, resources, examples, workflows)

### Improved
- Enhanced tool descriptions with detailed parameter explanations
- Better discovery of MCP capabilities for AI assistants
- More helpful error messages and guidance in developer tools

## [0.12.1] - 2025-07-13

### Fixed
- Prevent duplicate MCP configuration messages when using WithMCPSupport with MCPDev()
- Auto-configuration now correctly skips when MCP is already configured programmatically

### Added
- Test coverage for MCP configuration scenarios to prevent regression

## [0.12.0] - 2025-07-13

### Added
- MCP configuration via command-line flags and environment variables
  - `--mcp`, `--mcp-dev`, `--mcp-observability`, `--mcp-transport` flags
  - `HS_MCP_*` environment variables for all MCP settings
- Auto-configuration of MCP from ServerOptions during server initialization
- Claude Code integration examples with HTTP transport
- Comprehensive MCP flags example showing different configuration methods

### Changed
- MCP can now be configured without hardcoding in source code
- Updated documentation to emphasize flag/environment configuration over code
- Enhanced README with Claude Code HTTP integration examples

### Security
- Development mode (`MCPDev()`) no longer needs to be hardcoded in production builds

## [0.11.0] - 2025-07-13

### Added
- MCP Developer Tools (`MCPDev()`) for AI-assisted development
  - Server restart and reload capabilities
  - Dynamic log level changes
  - Route inspection
  - HTTP request capture and replay
- MCP Observability (`MCPObservability()`) for production monitoring
  - Sanitized server configuration
  - Health metrics and uptime
  - Recent logs with circular buffer
- MCP Extensions API for building custom tools and resources
  - Fluent builder pattern
  - `SimpleTool` and `SimpleResource` helpers
  - `MCPExtension` for grouping functionality
- Comprehensive MCP Integration Guide
- DevOps support with environment variables
  - `HS_DEBUG` and `HS_LOG_LEVEL` for logging control
  - `WithDebugMode()` and `WithLogLevel()` options

### Changed
- **BREAKING**: MCP support now requires transport configuration (HTTP or STDIO)
- **BREAKING**: MCP built-in tools and resources now disabled by default
- Restructured MCP API for better separation of concerns
- Improved README with professional tone and Claude Desktop examples

### Security
- MCP DevOps resources explicitly exclude sensitive data
- Developer mode shows prominent warning in logs

## [0.10.0] - 2025-07-13

### Added
- WebSocket support (RFC 6455 compliant)
  - Zero-dependency implementation using standard library
  - Secure-by-default with origin validation
  - Configurable frame size limits
  - Ping/pong keepalive support
- WebSocket security features
  - Origin validation with `CheckOrigin`
  - `SameOriginCheck()` and `AllowedOriginsCheck()` helpers
  - Subprotocol negotiation
  - Extension support hooks
- Enhanced middleware compatibility
  - ResponseWriter interface preservation (Hijacker, Flusher, etc.)
  - Proper error handling in middleware chains
- Comprehensive WebSocket guide and examples

### Security
- WebSocket frame validation per RFC 6455
- Protection against frame injection attacks
- Secure defaults for origin checking

[0.13.3]: https://github.com/osauer/hyperserve/compare/v0.13.2...v0.13.3
[0.13.2]: https://github.com/osauer/hyperserve/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/osauer/hyperserve/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/osauer/hyperserve/compare/v0.12.2...v0.13.0
[0.12.2]: https://github.com/osauer/hyperserve/compare/v0.12.1...v0.12.2
[0.12.1]: https://github.com/osauer/hyperserve/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/osauer/hyperserve/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/osauer/hyperserve/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/osauer/hyperserve/compare/v0.9.0...v0.10.0
