package server

/*
Built-in middleware for common HTTP server functionality.

The middleware package includes:
  - Request logging with structured output
  - Panic recovery to prevent server crashes
  - Request metrics collection
  - Rate limiting per IP address
  - Security headers (HSTS, CSP, etc.)
  - Request/Response timing

Middleware can be applied globally with Server.Use or to a path subtree with
Server.UsePrefix. Authentication is provided by pkg/auth, where it can remain
independent of server configuration.
*/

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// Middleware wraps an HTTP handler. It has the standard net/http middleware
// shape, so middleware from other packages works without an adapter.
type Middleware func(http.Handler) http.Handler

// MiddlewareStack is a collection of middleware functions that can be applied to an http.Handler.
// Middleware in the stack is applied in order, with the first middleware being the outermost.
type MiddlewareStack []Middleware

const globalMiddlewareRoute = "*"

// middlewareRegistry manages middleware stacks for different routes.
//
// `sortedRoutes` is a precomputed view of non-global route keys, ordered by
// ascending length (ties alphabetical), so more-specific prefixes wrap the
// handler more tightly. It is rebuilt only inside Add(); the request hot
// path reads it and never allocates a key slice or runs a sort.
//
// The registry is an implementation detail; callers compose middleware with
// Server.Use and Server.UsePrefix.
type middlewareRegistry struct {
	middleware   map[string]MiddlewareStack
	sortedRoutes []string
	frozen       atomic.Bool
}

// middlewareDispatcher delays compilation until the first request. That keeps
// Handler construction compatible with middleware registered before serving,
// while sync.Once makes the resulting request plan immutable.
type middlewareDispatcher struct {
	registry *middlewareRegistry
	mux      *http.ServeMux
	once     sync.Once
	handler  http.Handler
}

// compiledMiddlewareNode is one prefix in the immutable dispatch tree. Its
// middleware wraps the node exactly once; ServeHTTP selects the most-specific
// matching child or falls through to the standard library mux.
type compiledMiddlewareNode struct {
	prefix   string
	children []*compiledMiddlewareNode
	fallback http.Handler
	handler  http.Handler
}

// newMiddlewareRegistry creates a new registry with optional global middleware.
// If globalMiddleware is provided, it will be applied to all routes by default.
func newMiddlewareRegistry(globalMiddleware MiddlewareStack) *middlewareRegistry {
	ret := &middlewareRegistry{
		middleware: make(map[string]MiddlewareStack),
	}
	if globalMiddleware != nil {
		ret.Add(globalMiddlewareRoute, globalMiddleware)
	}
	return ret
}

// applyToMux returns a dispatcher that compiles middleware on its first request
// and is backed by the standard library mux. Prefix matching remains
// segment-aware: `/api` matches `/api/users`, not `/apiv2`.
func (mwr *middlewareRegistry) applyToMux(mux *http.ServeMux) http.Handler {
	return &middlewareDispatcher{registry: mwr, mux: mux}
}

func (mwr *middlewareRegistry) compile(mux *http.ServeMux) http.Handler {
	mwr.frozen.Store(true)
	root := &compiledMiddlewareNode{fallback: mux}
	nodes := make([]*compiledMiddlewareNode, 0, len(mwr.sortedRoutes))

	// Every node attaches to its nearest registered prefix. The resulting tree
	// means a middleware factory is invoked once for its registration site, so
	// state in standard third-party middleware is shared across child paths.
	for _, prefix := range mwr.sortedRoutes {
		parent := root
		for _, candidate := range nodes {
			if pathPrefixMatches(prefix, candidate.prefix) {
				parent = candidate
			}
		}

		node := &compiledMiddlewareNode{
			prefix:   prefix,
			fallback: mux,
		}
		node.handler = applyMiddlewareStack(mwr.middleware[prefix], node)
		parent.children = append(parent.children, node)
		nodes = append(nodes, node)
	}

	root.handler = applyMiddlewareStack(mwr.middleware[globalMiddlewareRoute], root)
	return root.handler
}

func applyMiddlewareStack(stack MiddlewareStack, next http.Handler) http.Handler {
	for _, middleware := range slices.Backward(stack) {
		next = middleware(next)
	}
	return next
}

func (d *middlewareDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.once.Do(func() {
		d.handler = d.registry.compile(d.mux)
	})
	d.handler.ServeHTTP(w, r)
}

func (node *compiledMiddlewareNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, child := range slices.Backward(node.children) {
		if pathPrefixMatches(r.URL.Path, child.prefix) {
			child.handler.ServeHTTP(w, r)
			return
		}
	}
	node.fallback.ServeHTTP(w, r)
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

// Add registers a middleware stack for one internal route key.
func (mwr *middlewareRegistry) Add(route string, middleware MiddlewareStack) {
	if mwr.frozen.Load() {
		panic("hyperserve: middleware registered after serving started")
	}
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
		if k == globalMiddlewareRoute {
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
		ret = MiddlewareStack{}
	}
	return ret
}

func defaultMiddleware(server *Server) MiddlewareStack {
	return MiddlewareStack{
		MetricsMiddleware(server),
		requestLoggerMiddleware(server.logger),
		recoveryMiddleware(server.logger)}
}

// SecureWeb returns security-header middleware for browser-facing routes.
// Pass the Options snapshot from the Server whose TLS, CSP, CORS, and optional
// Server header policy should be applied.
func SecureWeb(options Options) Middleware {
	return HeadersMiddleware(options)
}

// middleware definitions

// header is an internal key/value pair used by the static securityHeaders
// table. It is intentionally unexported — the previous public Header type
// had unexported fields, making it impossible for callers to construct one.
type header struct {
	key   string
	value string
}

// MetricsMiddleware returns a middleware function that collects request metrics.
// It tracks total request count and response times for performance monitoring.
func MetricsMiddleware(srv *Server) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			srv.totalRequests.Add(1)
			start := time.Now()
			next.ServeHTTP(w, r)
			srv.totalResponseTime.Add(time.Since(start).Microseconds())
		})
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
func RequestLoggerMiddleware(next http.Handler) http.Handler {
	return requestLoggerMiddleware(slog.Default())(next)
}

func requestLoggerMiddleware(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Logger.Info uses context.Background internally, so use the same
			// context for the fast-path decision to preserve custom-handler
			// Enabled semantics.
			if !logger.Enabled(context.Background(), slog.LevelInfo) {
				next.ServeHTTP(w, r)
				return
			}
			lrw := &loggingResponseWriter{w, http.StatusOK, 0}
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			start := time.Now()
			next.ServeHTTP(lrw, r)
			logger.Info("Request completed",
				"from", ip,
				"method", r.Method,
				"url", r.URL.String(),
				"status", lrw.statusCode,
				"bytes", lrw.bytesWritten,
				"duration", time.Since(start))
		})
	}
}

// RecoveryMiddleware returns a middleware function that recovers from panics in request handlers.
// Catches panics, logs the error, and returns a 500 Internal Server Error response.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return recoveryMiddleware(slog.Default())(next)
}

func recoveryMiddleware(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("Panic recovered", "error", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware returns a middleware function that enforces rate limiting per client IP address.
// Uses token bucket algorithm with configurable rate limit and burst capacity.
// Returns 429 Too Many Requests when rate limit is exceeded.
// Optimized for Go 1.24's Swiss Tables map implementation.
func RateLimitMiddleware(srv *Server) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						limiter: rate.NewLimiter(srv.options.RateLimit, srv.options.Burst),
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
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", float64(srv.options.RateLimit)))
				w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", entry.limiter.Tokens()))
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Second).Unix()))
				next.ServeHTTP(w, r)
			} else {
				// Add retry-after header for better client behavior
				w.Header().Set("Retry-After", "1")
				writeErrorResponse(w, http.StatusTooManyRequests, "Rate limit exceeded")
			}
		})
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
func generateCSP(options Options) string {
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
func HeadersMiddleware(options Options) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		})
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
