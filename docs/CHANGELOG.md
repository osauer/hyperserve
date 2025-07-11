# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **Health Server**: Now disabled by default to avoid port conflicts
  - Previously defaulted to port 9080 which could cause conflicts
  - Enable explicitly with `WithHealthServer()` option
- **Logging Improvements**: Reduced startup verbosity
  - JSON-RPC method registration now logs at DEBUG level instead of INFO
  - Various initialization messages moved to DEBUG level
  - Consolidated server startup message shows key configuration

### Fixed
- Health server port conflicts when running multiple instances
- Excessive logging during MCP initialization

## [0.9.3] - 2025-07-11

### Added
- **MCP GET Request Support**: The MCP endpoint now returns helpful documentation when accessed via GET request
- **Granular MCP Control**: New options to control built-in tools and resources separately
  - `WithMCPBuiltinTools(bool)` - Enable/disable built-in tools
  - `WithMCPBuiltinResources(bool)` - Enable/disable built-in resources
  - Environment variables: `HS_MCP_TOOLS_ENABLED`, `HS_MCP_RESOURCES_ENABLED`

### Changed
- **BREAKING**: MCP built-in tools and resources are now **disabled by default** for security
  - Users must explicitly enable them using `WithMCPBuiltinTools(true)` and/or `WithMCPBuiltinResources(true)`
  - This change improves security by ensuring users opt-in to functionality they need
- Consolidated MCP optimizations from separate file into main mcp.go file

### Deprecated
- `WithMCPToolsDisabled()` - Use `WithMCPBuiltinTools(false)` instead
- `WithMCPResourcesDisabled()` - Use `WithMCPBuiltinResources(false)` instead

### Fixed
- Calculator tool now properly handles infinity/NaN results to prevent JSON marshaling errors
- MCP integration tests adjusted for new default behavior

## [0.9.2] - 2025-07-11

### Added
- **MCP Performance Optimizations**: Enhanced Model Context Protocol with significant performance improvements
  - Context support for tools enabling cancellation and timeouts (30s default)
  - Resource caching with configurable TTL (5 minutes default) 
  - Comprehensive metrics collection for requests, tools, and resources
  - Thread-safe concurrent tool execution
  - Cache hit/miss tracking and performance metrics
- **MCP Metrics API**: New `GetMetrics()` method provides detailed performance insights
  - Request counts and latencies by method
  - Tool execution statistics with error rates
  - Resource read performance and cache effectiveness
  - Overall error rates and throughput metrics

### Changed
- MCP tools now support context for better resource management
- MCP resources are automatically cached to reduce redundant reads
- All MCP operations now collect performance metrics

### Fixed
- Middleware registration logs now only appear during setup, not on every request
- Fixed middleware log spam that was impacting performance

## [0.9.1] - 2025-07-10

### Added
- **Custom MCP Tools and Resources**: Added support for registering custom Model Context Protocol extensions
  - `RegisterMCPTool()` - Register custom tools after server creation
  - `RegisterMCPResource()` - Register custom resources after server creation
  - `MCPEnabled()` - Check if MCP support is enabled
  - Updated documentation and examples showing custom tool implementation
- **Benchmarking Suite**: Comprehensive performance testing framework
  - Memory efficiency: ~1KB per request with 10 allocations
  - Middleware overhead: 10-30% for full security stack
  - Individual middleware measurements show relative costs
  - Hardware-independent metrics focus on efficiency
- **Performance Documentation**: Added detailed analysis with focus on relative performance
- **Architecture Decision Records (ADRs)**: Documented 11 key architecture decisions
  - Minimal external dependencies strategy
  - Functional options configuration pattern
  - Layered middleware architecture
  - Configuration precedence hierarchy
  - Separate health check server design
  - Go 1.24 minimum version requirement
  - Optional template system integration
  - Context-based graceful shutdown
  - Single package architecture choice
  - Server-Sent Events as first-class feature
  - Model Context Protocol (MCP) support

### Fixed
- **Route-specific middleware**: Fixed critical bug where all middleware was applied globally
  - Middleware registered for specific routes (e.g., `/api`) now only applies to matching paths
  - Global middleware (registered with `"*"`) correctly applies to all routes
  - Proper execution order: global middleware runs first, then route-specific
  - Multiple middleware for the same route are now appended instead of replaced
- **Logging level**: Changed options.json file not found message from WARN to INFO level
- **Examples**: Fixed all example compilation errors
  - Updated auth validator signatures to return (bool, error)
  - Fixed MCP tool interface implementation
  - Corrected SSE message helper usage
  - Fixed rate limit formatting in configuration example
- **Test failures**: Fixed all test issues including:
  - Unused imports in enterprise example
  - Template parsing with os.Root security
  - Middleware test integration
  - Health endpoint testing on correct server
  - Parallel test execution conflicts
- **Template directory modification**: Removed side effect in HandleTemplate that modified Options.TemplateDir
- **Options mutation bug**: Fixed issue where defaultServerOptions was being modified across tests

## [0.9.0] - 2025-06-27

### Added
- **FIPS 140-3 Compliance**: Added `WithFIPSMode()` option for government and enterprise deployments
- **Encrypted Client Hello (ECH)**: Added `WithEncryptedClientHello()` to encrypt SNI in TLS handshakes  
- **Post-Quantum Cryptography**: Automatic X25519MLKEM768 key exchange when not in FIPS mode
- **Timing Attack Protection**: Authentication now uses `crypto/subtle.WithDataIndependentTiming`
- **Secure File Serving**: Implemented `os.Root` for sandboxed directory access, preventing traversal attacks
- **Swiss Tables**: Rate limiting now uses Go 1.24's faster map implementation (30-35% improvement)
- **Graceful Shutdown**: Added `Stop()` method with 10-second timeout
- **Rate Limiter Cleanup**: Optimized cleanup using timestamp tracking instead of token counting
- **Comprehensive Documentation**: Added Go doc comments for all exported types and functions

### Changed
- **Minimum Go version** is now 1.24 (breaking change)
- **Rate limiter implementation** changed from `sync.Map` to regular map with RWMutex for better performance
- **SSE message formatting** optimized by removing redundant `fmt.Sprintf`
- **Security headers** updated to modern 2024 standards with CSP, CORS, and Cross-Origin policies
- **Chaos mode** default changed to `false` for production safety
- **Middleware signatures** updated to use server instances instead of global state
- **Config merging** now uses reflection-based automatic merging instead of manual field copying

### Deprecated
- None in this release

### Removed
- **Global state variables**: Moved `clientLimiters` and `requestCounter` to server instances

### Fixed
- **Nil pointer dereference** in shutdown when server not started
- **Test race conditions** by implementing parallel test execution safety
- **Memory leaks** from rate limiter accumulation with periodic cleanup mechanism
- **Template parsing errors** with improved error handling
- **Test function naming** to include `Test` prefix for proper execution

### Security
- **Enhanced authentication** with proper token validation framework
- **Modern security headers** including Permissions-Policy, COEP, COOP, CORP
- **Rate limit headers** added for better client guidance (`X-RateLimit-*`, `Retry-After`)
- **Directory traversal prevention** with `os.Root` sandboxing
- **Timing-safe comparisons** in authentication middleware

## [0.8.0] and earlier

See [RELEASE_NOTES.md](../RELEASE_NOTES.md) for detailed information about earlier releases.

---

## Version Support

- **v0.9.x**: Active development, pre-release stabilization
- **v1.0.0**: Planned stable release with API stability guarantees
- **Future versions**: Will follow strict semantic versioning

## Upgrade Guides

For detailed upgrade instructions between versions, see:
- [MIGRATION_GUIDE.md](../MIGRATION_GUIDE.md) - Go 1.24 features and migration
- [RELEASE_NOTES.md](../RELEASE_NOTES.md) - Detailed release notes with upgrade steps

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for information on how to contribute to this project.