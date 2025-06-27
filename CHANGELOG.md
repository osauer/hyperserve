# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Benchmarking Suite**: Comprehensive performance testing framework
  - Baseline: 2.6M+ requests/sec (381ns per request)
  - Secure API: 2.9M requests/sec with full middleware stack
  - Individual middleware overhead measurements
  - Memory allocation profiling
- **Performance Documentation**: Added benchmark results and analysis

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

See [RELEASE_NOTES.md](./RELEASE_NOTES.md) for detailed information about earlier releases.

---

## Version Support

- **v0.9.x**: Active development, pre-release stabilization
- **v1.0.0**: Planned stable release with API stability guarantees
- **Future versions**: Will follow strict semantic versioning

## Upgrade Guides

For detailed upgrade instructions between versions, see:
- [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md) - Go 1.24 features and migration
- [RELEASE_NOTES.md](./RELEASE_NOTES.md) - Detailed release notes with upgrade steps

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for information on how to contribute to this project.