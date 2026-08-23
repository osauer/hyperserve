package server

/*
Built-in middleware for common HTTP server functionality.

The middleware package includes:
  - Request logging with structured output
  - Panic recovery to prevent server crashes
  - Request metrics collection
  - Authentication (Basic, Bearer token, custom)
  - Rate limiting per IP address
  - Security headers (HSTS, CSP, etc.)
  - Request/Response timing

Middleware can be applied globally or to specific routes:

	// Global middleware
	srv.AddMiddleware("*", server.RequestLoggerMiddleware)

	// Route-specific middleware
	srv.AddMiddleware("/api", server.AuthMiddleware(srv.Options))

	// Combine multiple middleware
	srv.AddMiddlewareGroup("/admin",
		server.AuthMiddleware(srv.Options),
		server.RateLimitMiddleware(srv),
	)
*/

import (
	"bufio"
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// MiddlewareFunc is a function type that wraps an http.Handler and returns a new http.HandlerFunc.
// This is the standard pattern for HTTP middleware in Go.
type MiddlewareFunc func(http.Handler) http.HandlerFunc

// MiddlewareStack is a collection of middleware functions that can be applied to an http.Handler.
// Middleware in the stack is applied in order, with the first middleware being the outermost.
type MiddlewareStack []MiddlewareFunc

// GlobalMiddlewareRoute is a special route identifier that applies middleware to all routes.
// Use this constant when registering middleware that should run for every request.
const GlobalMiddlewareRoute = "*"

// middlewareRegistry manages middleware stacks for different routes.
//
// `sortedRoutes` is a precomputed view of non-global route keys, ordered by
// ascending length (ties alphabetical), so more-specific prefixes wrap the
// handler more tightly. It is rebuilt only inside Add(); the request hot
// path reads it and never allocates a key slice or runs a sort.
//
// Unexported in v1.0 — the type had no external callers and the field
// holding it on Server was already unexported. Use AddMiddlewareStack on
// the server to compose routes.
type middlewareRegistry struct {
	middleware   map[string]MiddlewareStack
	sortedRoutes []string
}

// newMiddlewareRegistry creates a new registry with optional global middleware.
// If globalMiddleware is provided, it will be applied to all routes by default.
func newMiddlewareRegistry(globalMiddleware MiddlewareStack) *middlewareRegistry {
	ret := &middlewareRegistry{
		middleware: make(map[string]MiddlewareStack),
	}
	if globalMiddleware != nil {
		ret.Add(GlobalMiddlewareRoute, globalMiddleware)
	}
	return ret
}

// applyToMux returns an http.Handler that chains the global stack ("*") with
// every route-specific stack whose key is a path-segment prefix of the
// request path. "Path-segment prefix" means the key matches at a `/`
// boundary: a key of `/api` matches `/api`, `/api/`, and `/api/foo`, but
// not `/api2/foo` or `/apifoo`. The previous `strings.HasPrefix`-only
// check made the latter two fire the `/api` middleware too — a real but
// rare correctness bug, fixed here ahead of the v1.0 freeze.
//
// The route ordering (ascending length, ties alphabetical) is fixed at
// Add() time so the request path does no sorting and allocates no key
// slice. Only the per-middleware closures (intrinsic to the design)
// allocate per request.
func (mwr *middlewareRegistry) applyToMux(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		finalHandler := http.Handler(mux)

		// Walk routes from longest to shortest so deeper prefixes wrap
		// closest to the mux. sortedRoutes is ascending, so we iterate in
		// reverse; the inner stack also wraps in reverse so handler[0]
		// is the outermost middleware in its stack.
		for _, key := range slices.Backward(mwr.sortedRoutes) {

			if !pathPrefixMatches(path, key) {
				continue
			}
			stack := mwr.middleware[key]
			for _, s := range slices.Backward(stack) {
				finalHandler = s(finalHandler)
			}
		}

		// Global stack wraps everything (outermost).
		global := mwr.middleware[GlobalMiddlewareRoute]
		for _, g := range slices.Backward(global) {
			finalHandler = g(finalHandler)
		}

		finalHandler.ServeHTTP(w, r)
	})
}

// pathPrefixMatches reports whether `key` is a path-segment prefix of
// `path`. The match rules:
//
//   - key == ""                            → match (universal: empty is a
//     prefix of every path. Some callers
//     use "" interchangeably with "*"
//     for "apply to all routes".)
//   - key == path                          → match (exact)
//   - key has a trailing slash             → strings.HasPrefix is sufficient
//     (key was registered as "/api/", so "/api/anything" is in scope)
//   - key has no trailing slash and the
//     next character in path is '/'        → match (e.g. key "/api" vs
//     path "/api/foo")
//   - otherwise                            → no match (e.g. key "/api" vs
//     path "/api2/foo")
//
// The "/" key behaves as a universal prefix because every path begins
// with "/", which is the documented intent of the root middleware key.
func pathPrefixMatches(path, key string) bool {
	if key == "" {
		return true
	}
	if !strings.HasPrefix(path, key) {
		return false
	}
	if len(path) == len(key) {
		return true
	}
	// path is strictly longer than key. The trailing-slash case is
	// already a match (key="/api/", path="/api/foo": next char is 'f',
	// but the slash boundary is part of key itself). For keys without a
	// trailing slash we require the next path char to be '/' so that
	// "/api" doesn't claim "/api2".
	if key[len(key)-1] == '/' {
		return true
	}
	return path[len(key)] == '/'
}

// Add registers a MiddlewareStack for a specific route in the registry.
// Use GlobalMiddlewareRoute ("*") to apply middleware to all routes.
func (mwr *middlewareRegistry) Add(route string, middleware MiddlewareStack) {
	if existing, exists := mwr.middleware[route]; exists {
		// Append to existing middleware for this route
		mwr.middleware[route] = append(existing, middleware...)
	} else {
		// Create new entry
		mwr.middleware[route] = middleware
	}
	mwr.rebuildSorted()
}

// rebuildSorted refreshes the cached route ordering after a mutation. Add()
// is the only mutator of the registry; tests with direct map access exist
// but never read sortedRoutes, so calling this here is sufficient.
func (mwr *middlewareRegistry) rebuildSorted() {
	keys := make([]string, 0, len(mwr.middleware))
	for k := range mwr.middleware {
		if k == GlobalMiddlewareRoute {
			continue
		}
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if d := len(a) - len(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	mwr.sortedRoutes = keys
}

// Get retrieves the MiddlewareStack for a specific route.
// Returns an empty MiddlewareStack if no middleware is registered for the route.
func (mwr *middlewareRegistry) Get(route string) MiddlewareStack {
	ret := mwr.middleware[route]
	if ret == nil {
		logger.Warn("No middleware found for route", "route", route)
		ret = MiddlewareStack{}
	}
	return ret
}

// DefaultMiddleware returns a predefined middleware stack with essential server functionality.
// Includes metrics collection, request logging, and panic recovery.
// This middleware is applied by default unless explicitly excluded.
func DefaultMiddleware(server *Server) MiddlewareStack {
	return MiddlewareStack{
		MetricsMiddleware(server),
		RequestLoggerMiddleware,
		RecoveryMiddleware}
}

// SecureAPI returns a middleware stack configured for secure API endpoints.
// Includes authentication and rate limiting middleware.
func SecureAPI(srv *Server) MiddlewareStack {
	return MiddlewareStack{
		AuthMiddleware(srv.Options),
		RateLimitMiddleware(srv)}
}

// SecureWeb returns a middleware stack configured for secure web endpoints.
// Includes security headers middleware for web applications.
func SecureWeb(options *ServerOptions) MiddlewareStack {
	return MiddlewareStack{HeadersMiddleware(options)}
}

// middleware definitions

// Header context keys
type contextKey string

const (
	authorizationHeader            = "Authorization"
	bearerTokenPrefix              = "Bearer "
	sessionIDKey        contextKey = "sessionID"
)

// header is an internal key/value pair used by the static securityHeaders
// table. It is intentionally unexported — the previous public Header type
// had unexported fields, making it impossible for callers to construct one.
type header struct {
	key   string
	value string
}

// MetricsMiddleware returns a middleware function that collects request metrics.
// It tracks total request count and response times for performance monitoring.
func MetricsMiddleware(srv *Server) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			srv.totalRequests.Add(1)
			start := time.Now()
			next.ServeHTTP(w, r)
			srv.totalResponseTime.Add(time.Since(start).Microseconds())
		}
	}
}

// AuthMiddleware returns a middleware function that validates bearer tokens in the Authorization header.
// Requires requests to include a valid Bearer token, otherwise returns 401 Unauthorized.
func AuthMiddleware(options *ServerOptions) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Check for auth token
			authHeader := r.Header.Get(authorizationHeader)

			// check if header has bearer token
			token, ok := strings.CutPrefix(authHeader, bearerTokenPrefix)
			if !ok {
				http.Error(w, "Unauthorized: Bearer token required", http.StatusUnauthorized)
				return
			}
			if token == "" {
				http.Error(w, "Unauthorized: Bearer token invalid", http.StatusUnauthorized)
				return
			}

			// validate token with timing attack protection
			if options.AuthTokenValidatorFunc == nil {
				http.Error(w, "Internal Server Error: Auth not configured", http.StatusInternalServerError)
				return
			}

			// Use crypto/subtle.WithDataIndependentTiming for constant-time token validation
			var valid bool
			var err error
			subtle.WithDataIndependentTiming(func() {
				valid, err = options.AuthTokenValidatorFunc(token)
			})

			if err != nil {
				logger.Error("error validating token", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if !valid {
				http.Error(w, "Unauthorized: Bearer token invalid", http.StatusUnauthorized)
				return
			}

			// add session and ID to the context
			ctx := context.WithValue(r.Context(), sessionIDKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequestLoggerMiddleware returns a middleware function that logs structured request information.
// It captures and logs:
//   - Client IP address
//   - HTTP method and URL path
//   - Trace ID (if present in X-Trace-ID header)
//   - Response status code
//   - Request duration
//   - Response size in bytes
//
// This middleware is included by default in NewServer().
// For high-traffic applications, consider the performance impact of logging.
func RequestLoggerMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// create a new logging response writer to capture status code and bytes written
		lrw := &loggingResponseWriter{w, http.StatusOK, 0}

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)

		start := time.Now()
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)
		logger.Info("Request completed",
			"from", ip,
			"method", r.Method,
			"url", r.URL.String(),
			"status", lrw.statusCode,
			"bytes", lrw.bytesWritten,
			"duration", duration)
	}
}

// RecoveryMiddleware returns a middleware function that recovers from panics in request handlers.
// Catches panics, logs the error, and returns a 500 Internal Server Error response.
func RecoveryMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Panic recovered", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	}
}

// RateLimitMiddleware returns a middleware function that enforces rate limiting per client IP address.
// Uses token bucket algorithm with configurable rate limit and burst capacity.
// Returns 429 Too Many Requests when rate limit is exceeded.
// Optimized for Go 1.24's Swiss Tables map implementation.
func RateLimitMiddleware(srv *Server) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)

			// Try to get existing limiter with read lock (fast path)
			srv.rateLimiters.mu.RLock()
			entry, exists := srv.rateLimiters.clients[ip]
			srv.rateLimiters.mu.RUnlock()

			if !exists {
				// Create new limiter with write lock
				srv.rateLimiters.mu.Lock()
				// Double-check in case another goroutine created it
				entry, exists = srv.rateLimiters.clients[ip]
				if !exists {
					entry = &rateLimiterEntry{
						limiter: rate.NewLimiter(srv.Options.RateLimit, srv.Options.Burst),
					}
					entry.lastAccessUnixNano.Store(time.Now().UnixNano())
					srv.rateLimiters.clients[ip] = entry
				}
				srv.rateLimiters.mu.Unlock()
			} else {
				// Hot-path timestamp bump — atomic store, no second lock.
				entry.lastAccessUnixNano.Store(time.Now().UnixNano())
			}

			if entry.limiter.Allow() {
				// Add rate limit headers to inform clients of their current status
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", float64(srv.Options.RateLimit)))
				w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", entry.limiter.Tokens()))
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Second).Unix()))
				next.ServeHTTP(w, r)
			} else {
				// Add retry-after header for better client behavior
				w.Header().Set("Retry-After", "1")
				writeErrorResponse(w, http.StatusTooManyRequests, "Rate limit exceeded")
			}
		}
	}
}

// securityHeaders provide headers for HeadersMiddleware.
//
// Strict-Transport-Security is intentionally omitted from this table: HSTS
// over plaintext is at best meaningless and at worst harmful (a reverse-proxy
// terminating TLS in front of us would inherit `preload` against intent), so
// the header is only set when EnableTLS is true. See HeadersMiddleware below.
//
// Access-Control-* headers are intentionally NOT in this table. The CORS
// contract belongs to applyCORSHeaders, which honours WithCORS configuration;
// emitting them unconditionally created a footgun where a handler echoing
// Origin into Access-Control-Allow-Origin would combine with a static
// Access-Control-Allow-Credentials: true to produce a credentialed wildcard.
var securityHeaders = []header{
	{"X-Content-Type-Options", "nosniff"},                  // Prevent MIME-type sniffing
	{"X-Frame-Options", "DENY"},                            // Mitigate clickjacking
	{"Referrer-Policy", "strict-origin-when-cross-origin"}, // Balance privacy and functionality
	{"Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), fullscreen=(self)"}, // Modern replacement for Feature-Policy (removed invalid 'speaker' directive)
	{"Cross-Origin-Embedder-Policy", "require-corp"}, // Prevent cross-origin attacks
	{"Cross-Origin-Opener-Policy", "same-origin"},    // Isolate browsing context
	{"Cross-Origin-Resource-Policy", "same-origin"},  // Control cross-origin resource sharing
	{"X-Permitted-Cross-Domain-Policies", "none"},    // Restrict Flash/PDF cross-domain access
}

// generateCSP generates a Content Security Policy header value based on server
// options. The directive set was previously duplicated as two near-identical
// string literals — the two-form variant tolerated drift on a
// security-critical header. Now both forms are derived from one slice.
func generateCSP(options *ServerOptions) string {
	directives := []string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"media-src 'self'",
		"object-src 'none'",
	}
	if options.CSPWebWorkerSupport {
		directives = append(directives,
			"child-src 'self' blob:",
			"worker-src 'self' blob:",
		)
	} else {
		directives = append(directives, "child-src 'self'")
	}
	directives = append(directives,
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	)
	return strings.Join(directives, "; ")
}

// HeadersMiddleware returns a middleware function that adds security headers to responses.
// Includes headers for XSS protection, content type sniffing prevention, HSTS, CSP, and CORS.
// Automatically handles CORS preflight requests.
func HeadersMiddleware(options *ServerOptions) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Library consumers are unidentified by default. Applications that
			// deliberately opt in control the exact public value.
			if options.ServerHeader != "" {
				w.Header().Set("Server", options.ServerHeader)
			}

			// Set static security headers
			for _, h := range securityHeaders {
				w.Header().Set(h.key, h.value)
			}

			// Set dynamic CSP based on configuration
			w.Header().Set("Content-Security-Policy", generateCSP(options))

			// HSTS only over TLS. Two years + includeSubDomains + preload is
			// the value the Chrome HSTS preload list documents as the minimum
			// for inclusion; we ship it because anyone enabling EnableTLS in
			// this server is opting into "this hostname is HTTPS, period".
			if options.EnableTLS {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			}

			if handled := applyCORSHeaders(w, r, options.CORS); handled {
				return
			}

			// call the next handler if not in preflight
			next.ServeHTTP(w, r)
		}
	}
}

func applyCORSHeaders(w http.ResponseWriter, r *http.Request, cors *CORSOptions) bool {
	if cors == nil {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		return false
	}

	origin := r.Header.Get("Origin")
	allowedOrigin, originOK := cors.resolveAllowedOrigin(origin)

	if originOK {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		if allowedOrigin != "*" {
			addVaryHeader(w, "Origin")
		}
	} else {
		w.Header().Del("Access-Control-Allow-Origin")
	}

	if cors.AllowCredentials && originOK {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Del("Access-Control-Allow-Credentials")
	}

	if len(cors.AllowedMethods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", joinTokens(cors.AllowedMethods))
	} else {
		// Sensible default when CORS is configured but methods weren't
		// listed. Without this, preflight responds with no method header
		// and the browser blocks the request.
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
	if len(cors.AllowedHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", joinTokens(cors.AllowedHeaders))
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	if len(cors.ExposeHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", joinTokens(cors.ExposeHeaders))
	} else {
		w.Header().Del("Access-Control-Expose-Headers")
	}

	if cors.MaxAgeSeconds > 0 {
		w.Header().Set("Access-Control-Max-Age", formatMaxAge(cors.MaxAgeSeconds))
	}

	addVaryHeader(w, "Access-Control-Request-Method")
	addVaryHeader(w, "Access-Control-Request-Headers")

	if r.Method == http.MethodOptions {
		if origin == "" {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		if !originOK {
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	return false
}

func addVaryHeader(w http.ResponseWriter, value string) {
	if value == "" {
		return
	}
	existing := w.Header().Values("Vary")
	for _, header := range existing {
		for token := range strings.SplitSeq(header, ",") {
			if strings.EqualFold(strings.TrimSpace(token), value) {
				return
			}
		}
	}
	w.Header().Add("Vary", value)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (lrw *loggingResponseWriter) Flush() {
	flusher, ok := lrw.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bytesWritten += n
	return n, err
}

func (lrw *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return lrw.ResponseWriter
}

// Hijack implements the http.Hijacker interface to support WebSocket upgrades.
// It delegates to the underlying ResponseWriter if it implements http.Hijacker.
func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := lrw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

// ReadFrom implements io.ReaderFrom to optimize static file serving
func (lrw *loggingResponseWriter) ReadFrom(r io.Reader) (n int64, err error) {
	rf, ok := lrw.ResponseWriter.(io.ReaderFrom)
	if !ok {
		// Fall back to default behavior
		return io.Copy(lrw, r)
	}
	n, err = rf.ReadFrom(r)
	lrw.bytesWritten += int(n)
	return n, err
}

// Push implements http.Pusher for HTTP/2 server push support
func (lrw *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := lrw.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}
