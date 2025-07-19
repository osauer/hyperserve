# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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