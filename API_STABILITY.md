# API Stability Promise

HyperServe is committed to providing a stable, reliable API that developers can depend on for production applications.

## Semantic Versioning Commitment

Starting with **v1.0.0**, HyperServe follows strict [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

- ✅ **PATCH versions (1.0.X)** - Bug fixes only, no API changes
- ✅ **MINOR versions (1.X.0)** - New features, fully backward compatible  
- ⚠️ **MAJOR versions (X.0.0)** - May include breaking changes (rare and well-documented)

## Current Status: Pre-Release (v0.9.x)

**⚠️ Important**: Versions before v1.0.0 are considered pre-release and may include breaking changes between minor versions. However, we strive to minimize breaking changes and document them clearly.

## API Stability Guarantees (Starting v1.0.0)

### What Will NOT Change Within Major Versions

1. **Core API Signatures**
   ```go
   // These signatures are locked at v1.0.0
   func NewServer(opts ...ServerOptionFunc) (*Server, error)
   func (s *Server) Run() error
   func (s *Server) HandleFunc(pattern string, handler http.HandlerFunc)
   func (s *Server) AddMiddleware(pattern string, middleware ...MiddlewareFunc)
   ```

2. **Configuration Options**
   ```go
   // Existing WithX functions will remain stable
   WithAddr(addr string) ServerOptionFunc
   WithTLS(certFile, keyFile string) ServerOptionFunc
   WithTimeouts(read, write, idle time.Duration) ServerOptionFunc
   // ... all existing options
   ```

3. **Middleware System**
   ```go
   // Core middleware interfaces are stable
   type MiddlewareFunc func(http.Handler) http.HandlerFunc
   type MiddlewareStack []MiddlewareFunc
   ```

4. **Public Struct Fields**
   ```go
   type ServerOptions struct {
       Addr     string  // Will not be removed or change meaning
       Port     int     // Will not be removed or change meaning
       // ... existing fields remain stable
   }
   ```

### What May Change (Backward Compatible)

1. **New Configuration Options**
   ```go
   // New WithX functions may be added
   WithNewFeature(config FeatureConfig) ServerOptionFunc
   ```

2. **New Methods on Existing Types**
   ```go
   // New methods may be added to Server
   func (s *Server) NewMethod() error
   ```

3. **New Struct Fields**
   ```go
   type ServerOptions struct {
       Addr     string  // Existing fields stable
       Port     int     // Existing fields stable
       NewField string  `json:"new_field,omitempty"` // New fields added safely
   }
   ```

4. **Enhanced Middleware**
   ```go
   // New predefined middleware may be added
   func NewSecurityMiddleware() MiddlewareFunc
   ```

### What Would Require a Major Version (v2.0.0)

Breaking changes that would trigger a major version include:

- Removing public functions, methods, or struct fields
- Changing function signatures or return types
- Changing the meaning or behavior of existing configuration options
- Removing or renaming the module path

## Deprecation Policy

When we need to phase out functionality:

1. **Deprecation Warning** (Minor Version)
   ```go
   // Deprecated: Use NewBetterFunc instead. Will be removed in v2.0.0.
   func OldFunc() { /* redirects to new implementation */ }
   ```

2. **Continued Support** - Deprecated features continue working for the entire major version
3. **Migration Guide** - Clear documentation on how to migrate
4. **Removal** - Only in the next major version (e.g., v2.0.0)

## Examples of Stable vs. Breaking Changes

### ✅ Safe Changes (Minor/Patch Versions)

```go
// Adding optional configuration
func WithCaching(enabled bool) ServerOptionFunc { /* ... */ }

// Adding new methods
func (s *Server) GetMetrics() ServerMetrics { /* ... */ }

// Adding optional struct fields
type ServerOptions struct {
    Addr         string
    CacheEnabled bool `json:"cache_enabled,omitempty"` // Safe addition
}

// Enhancing existing functionality
func (s *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
    // Enhanced implementation, same behavior
}
```

### ❌ Breaking Changes (Major Version Required)

```go
// Changing function signatures
func NewServer(ctx context.Context, opts ...ServerOptionFunc) (*Server, error) // ❌

// Removing public methods
// func (s *Server) HandleFunc(...) // ❌ Removed

// Changing struct field types
type ServerOptions struct {
    Port string // ❌ Was int, now string
}

// Changing behavior significantly
func (s *Server) Run() error {
    // ❌ Now requires manual Start() call first
}
```

## Testing Compatibility

We maintain backward compatibility through:

1. **Automated Tests** - Every release includes compatibility tests
2. **Integration Tests** - Real-world usage scenarios
3. **Example Validation** - All documentation examples are tested
4. **Community Feedback** - Beta releases for major changes

## Getting Help

If you encounter what appears to be a breaking change in a minor/patch version:

1. **Check the [CHANGELOG.md](./CHANGELOG.md)** for documented changes
2. **Review [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md)** for upgrade instructions  
3. **File an issue** at https://github.com/osauer/hyperserve/issues
4. **Join discussions** at https://github.com/osauer/hyperserve/discussions

## Commitment Timeline

- **v0.9.x** (Current): Pre-release, minimal breaking changes
- **v1.0.0** (Planned): Full API stability guarantees begin
- **v1.x.x** (Future): Strict backward compatibility within major version
- **v2.0.0** (Future): Only if absolutely necessary for significant improvements

---

## Our Promise

**We understand that stability is crucial for production applications.** Starting with v1.0.0, you can upgrade HyperServe minor and patch versions with confidence that your existing code will continue to work without modification.

This API stability promise is our commitment to the HyperServe community.