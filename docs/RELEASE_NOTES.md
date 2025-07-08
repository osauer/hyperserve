# Release Notes

## Unreleased

### 🚀 New Features

#### Custom MCP Tools and Resources
- Added `RegisterMCPTool()` method to dynamically register custom MCP tools after server creation
- Added `RegisterMCPResource()` method to dynamically register custom MCP resources after server creation  
- Added `MCPEnabled()` method to check if MCP support is enabled on the server
- Updated MCP handler to support custom tool and resource registration
- Enhanced examples to demonstrate custom MCP extension implementation

### 📝 API Additions
```go
// MCP extension methods
func (s *Server) RegisterMCPTool(tool MCPTool) error
func (s *Server) RegisterMCPResource(resource MCPResource) error
func (s *Server) MCPEnabled() bool

// MCP interfaces
type MCPTool interface {
    Name() string
    Description() string
    Schema() map[string]interface{}
    Execute(params map[string]interface{}) (interface{}, error)
}

type MCPResource interface {
    URI() string
    Name() string
    Description() string
    MimeType() string
    Read(ctx context.Context) ([]byte, error)
}
```

### 📚 Documentation Updates
- Updated README.md with custom MCP tool and resource examples
- Added MCP interfaces to API stability guarantees
- Enhanced CLAUDE.md with custom MCP patterns

## v0.9.0 - Go 1.24 Update (2025-06-27)

### 🎯 Performance Characteristics

Key efficiency metrics:
- **Memory footprint**: ~1KB per request with full security stack
- **Allocation efficiency**: 10 allocations per request (no increase with middleware)
- **Middleware overhead**: Security features add only 10-30% to baseline latency
- **Zero-copy operations**: Minimal data copying in hot paths

Relative middleware costs:
- Recovery: -9% (optimizes request path)
- Auth: +21% (includes timing-safe token validation)
- Trace: +33% (UUID generation)
- RateLimit: +52% (Swiss Tables map lookup)
- RequestLogger: +254% (I/O bound)

### 🚀 Major Features

#### FIPS 140-3 Compliance
- Added `WithFIPSMode()` option for government and enterprise deployments
- Restricts TLS cipher suites to FIPS-approved algorithms only (AES-GCM)
- Limits elliptic curves to P256 and P384
- Enables compliance logging for audit trails

#### Enhanced Security
- **Encrypted Client Hello (ECH)**: Added `WithEncryptedClientHello()` to encrypt SNI in TLS handshakes
- **Post-Quantum Cryptography**: Automatically enables X25519MLKEM768 key exchange when not in FIPS mode
- **Timing Attack Protection**: Authentication now uses `crypto/subtle.WithDataIndependentTiming`
- **Secure File Serving**: Implemented `os.Root` for sandboxed directory access, preventing traversal attacks

#### Performance Improvements
- **Swiss Tables**: Rate limiting now uses Go 1.24's faster map implementation (30-35% improvement)
- **Optimized Cleanup**: Rate limiter cleanup uses timestamp tracking instead of token counting
- **Better Concurrency**: RWMutex for rate limiters reduces lock contention

### 🔧 Technical Changes

#### Breaking Changes
- Minimum Go version is now 1.24
- Rate limiter implementation changed from `sync.Map` to regular map with mutex

#### API Additions
```go
// New server options
WithFIPSMode() ServerOptionFunc
WithEncryptedClientHello(echKeys ...[]byte) ServerOptionFunc

// New server method
Stop() error  // Graceful shutdown with 10s timeout
```

#### Internal Improvements
- Added `rateLimiterEntry` struct for better cleanup tracking
- Template parsing now supports `os.Root` for secure file access
- Static file server uses custom handler when `os.Root` is available
- Fixed SSE message formatting (removed redundant `fmt.Sprintf`)

### 📝 Documentation Updates
- Updated README with Go 1.24 features section
- Added comprehensive examples for FIPS mode and ECH
- Documented performance optimizations
- Added security best practices

### 🐛 Bug Fixes
- Fixed nil pointer dereference in shutdown when server not started
- Fixed test race conditions
- Improved error handling in template parsing
- Added proper cleanup for os.Root handles

### 📦 Dependencies
- Updated to Go 1.24
- No new external dependencies added

### 🔮 Future Considerations
- WebAssembly support with `go:wasmexport` (planned)
- Enhanced metrics with build ID exposure (planned)
- Custom go vet checks for HyperServe patterns (planned)

---

## Upgrade Guide

### From v0.8.x to v0.9.0

1. **Update Go Version**
   ```bash
   # Install Go 1.24 or later
   go version  # Should show go1.24 or higher
   ```

2. **Update go.mod**
   ```go
   go 1.24
   ```

3. **Enable New Features (Optional)**
   ```go
   srv, err := hyperserve.NewServer(
       hyperserve.WithFIPSMode(),  // For FIPS compliance
       hyperserve.WithEncryptedClientHello(echKeys...),  // For ECH
   )
   ```

4. **Test Your Application**
   - Rate limiting behavior is unchanged but faster
   - File serving is more secure with os.Root
   - No API breaking changes for existing code

### Known Issues
- Some integration tests may fail due to middleware not being applied to direct mux access
- os.Root warnings appear in logs when directories don't exist (informational only)

### Support
Report issues at: https://github.com/osauer/hyperserve/issues