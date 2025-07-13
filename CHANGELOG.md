# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- **BREAKING**: Renamed `WithMCPSupport(name, version)` to require transport config
- **BREAKING**: MCP built-in tools and resources now disabled by default
- Improved README with professional tone and MCP examples

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

[0.11.0]: https://github.com/osauer/hyperserve/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/osauer/hyperserve/compare/v0.9.0...v0.10.0