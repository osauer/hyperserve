package hyperserve

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
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

// MiddlewareRegistry manages middleware stacks for different routes.
// It allows route-specific middleware configuration and supports exclusion of specific middleware.
type MiddlewareRegistry struct {
	middleware map[string]MiddlewareStack
	exclude    []MiddlewareFunc
}

// NewMiddlewareRegistry creates a new MiddlewareRegistry with optional global middleware.
// If globalMiddleware is provided, it will be applied to all routes by default.
func NewMiddlewareRegistry(globalMiddleware MiddlewareStack) *MiddlewareRegistry {
	ret := &MiddlewareRegistry{
		middleware: make(map[string]MiddlewareStack),
	}
	// add default middleware to all routes if defined in globalMiddleware stack
	if globalMiddleware != nil {
		ret.Add(GlobalMiddlewareRoute, globalMiddleware)
	}
	return ret
}

// Filter the MiddlewareRegistry based on include and exclude stacks
func (mwr *MiddlewareRegistry) filterMiddleware() {
	// range through the exclude middleware and remove them from the middleware registry
	for _, excl := range mwr.exclude {
		// range through all routes in the registry
		for key, mw := range mwr.middleware {
			filtered := MiddlewareStack{}
			for _, m := range mw {
				// we need to use reflect as Go as of 1.23 does not support direct comparison of func variables
				if reflect.ValueOf(m) != reflect.ValueOf(excl) {
					filtered = append(filtered, m)
				}
			}
			mwr.middleware[key] = filtered
		}
	}
}

// applyToMux helper to  apply multiple middleware to a handler
func (mwr *MiddlewareRegistry) applyToMux(mux *http.ServeMux) http.Handler {
	finalHandler := http.Handler(mux)
	mwr.filterMiddleware()
	for route, stack := range mwr.middleware {
		logger.Info("Applying middleware.", "route", route)
		// reverse order to run first MiddlewareFunc passed first
		for i := len(stack) - 1; i >= 0; i-- {
			finalHandler = stack[i](finalHandler)
		}
	}
	return finalHandler
}

// Add registers a MiddlewareStack for a specific route in the registry.
// Use GlobalMiddlewareRoute ("*") to apply middleware to all routes.
func (mwr *MiddlewareRegistry) Add(route string, middleware MiddlewareStack) {
	mwr.middleware[route] = middleware
}

// Get retrieves the MiddlewareStack for a specific route.
// Returns an empty MiddlewareStack if no middleware is registered for the route.
func (mwr *MiddlewareRegistry) Get(route string) MiddlewareStack {
	ret := mwr.middleware[route]
	if ret == nil {
		logger.Warn("No middleware found for route", "route", route)
		ret = MiddlewareStack{}
	}
	return ret
}

// RemoveStack removes all middleware for a specific route from the registry.
// Does nothing if no middleware is registered for the route.
func (mwr *MiddlewareRegistry) RemoveStack(route string) {
	delete(mwr.middleware, route)
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

// FileServer returns a middleware stack optimized for serving static files.
// Includes appropriate security headers for file serving.
func FileServer(options *ServerOptions) MiddlewareStack {
	return MiddlewareStack{HeadersMiddleware(options)}
}

// middleware definitions

// Header context keys
type contextKey string

const (
	authorizationHeader            = "Authorization"
	bearerTokenPrefix              = "Bearer "
	sessionIDKey        contextKey = "sessionID"
	traceIDKey          contextKey = "traceID"
)

// Header represents an HTTP header key-value pair used in middleware configuration.
type Header struct {
	key   string
	value string
}

// MetricsMiddleware returns a middleware function that collects request metrics.
// It tracks total request count and response times for performance monitoring.
func MetricsMiddleware(srv *Server) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		logger.Info("MetricsMiddleware enabled")
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
	logger.Info("AuthMiddleware enabled")
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Check for auth token
			authHeader := r.Header.Get(authorizationHeader)

			// check if header has bearer token
			if !strings.HasPrefix(authHeader, bearerTokenPrefix) {
				http.Error(w, "Unauthorized: Bearer token required", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, bearerTokenPrefix)
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

// RequestLoggerMiddleware returns a middleware function that logs detailed request information.
// Logs IP address, method, URL, trace ID, status code, and request duration.
// Use with caution as it may impact server performance.
func RequestLoggerMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("RequestLoggerMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		// create a new logging response writer to capture status code and bytes written
		lrw := &loggingResponseWriter{w, http.StatusOK, 0}

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		traceID := r.Context().Value(traceIDKey)
		if traceID == nil {
			traceID = ""
		}

		start := time.Now()
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)
		logger.Info("Request completed",
			"from", ip,
			"method", r.Method,
			"url", r.URL.String(),
			"trace_id", traceID,
			"status", lrw.statusCode,
			"duration", duration)
	}
}

// ResponseTimeMiddleware returns a middleware function that logs only the request duration.
// This is a lighter alternative to RequestLoggerMiddleware when only timing information is needed.
func ResponseTimeMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("ResponseTimeMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		logger.Info("Request duration", "duration", duration)
	}
}

// RecoveryMiddleware returns a middleware function that recovers from panics in request handlers.
// Catches panics, logs the error, and returns a 500 Internal Server Error response.
func RecoveryMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("RecoveryMiddleware enabled")
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
	logger.Info("RateLimitMiddleware enabled")
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			
			// Try to get existing limiter with read lock (fast path)
			srv.limitersMu.RLock()
			entry, exists := srv.clientLimiters[ip]
			srv.limitersMu.RUnlock()
			
			if !exists {
				// Create new limiter with write lock
				srv.limitersMu.Lock()
				// Double-check in case another goroutine created it
				entry, exists = srv.clientLimiters[ip]
				if !exists {
					entry = &rateLimiterEntry{
						limiter:    rate.NewLimiter(srv.Options.RateLimit, srv.Options.Burst),
						lastAccess: time.Now(),
					}
					srv.clientLimiters[ip] = entry
				}
				srv.limitersMu.Unlock()
			} else {
				// Update last access time
				srv.limitersMu.Lock()
				entry.lastAccess = time.Now()
				srv.limitersMu.Unlock()
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
			return
		}
	}
}

// securityHeaders provide headers for HeadersMiddleware
var securityHeaders = []Header{
	{"X-Content-Type-Options", "nosniff"},                                            // Prevent MIME-type sniffing
	{"X-Frame-Options", "DENY"},                                                      // Mitigate clickjacking
	{"Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload"},   // Enforce HTTPS with preload
	{"Referrer-Policy", "strict-origin-when-cross-origin"},                          // Balance privacy and functionality
	{"Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), speaker=(), fullscreen=(self)"}, // Modern replacement for Feature-Policy
	{"Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; media-src 'self'; object-src 'none'; child-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"}, // Comprehensive CSP
	{"Cross-Origin-Embedder-Policy", "require-corp"},                                // Prevent cross-origin attacks
	{"Cross-Origin-Opener-Policy", "same-origin"},                                   // Isolate browsing context
	{"Cross-Origin-Resource-Policy", "same-origin"},                                 // Control cross-origin resource sharing
	{"X-Permitted-Cross-Domain-Policies", "none"},                                   // Restrict Flash/PDF cross-domain access
	{"Access-Control-Allow-Methods", "GET, POST, OPTIONS"},                          // Allowed methods
	{"Access-Control-Allow-Headers", "Content-Type, Authorization"},                 // Allowed headers
	{"Access-Control-Allow-Credentials", "true"},                                    // If cookies or credentials are needed
	{"Access-Control-Max-Age", "600"},                                               // Pre-flight request cache
}

// HeadersMiddleware returns a middleware function that adds security headers to responses.
// Includes headers for XSS protection, content type sniffing prevention, HSTS, CSP, and CORS.
// Automatically handles CORS preflight requests.
func HeadersMiddleware(options *ServerOptions) MiddlewareFunc {
	logger.Info("HeadersMiddleware enabled")
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// todo implement hardened mode
			hardened := false
			if !hardened {
				w.Header().Set("Server", "hyperserve")
			}

			for _, h := range securityHeaders {
				w.Header().Set(h.key, h.value)
			}

			if options.EnableTLS {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}

			// ToDo add allowed site origin(s) to the header
			// w.Header().Set("Access-Control-Allow-Origin", "https://client-site.com")

			// Handle preflight request
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// call the next handler if not in preflight
			next.ServeHTTP(w, r)
		}
	}
}

// ChaosMiddleware returns a middleware handler that simulates random failures for chaos engineering.
// When chaos mode is enabled, can inject random latency, errors, throttling, and panics.
// Useful for testing application resilience and error handling.
func ChaosMiddleware(options *ServerOptions) MiddlewareFunc {
	logger.Info("ChaosMiddleware enabled")
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !options.ChaosMode {
				// Pass through if Chaos Mode is not enabled
				next.ServeHTTP(w, r)
				return
			}
			logger.Warn("Chaos Mode enabled")
			// Random latency

			if options.ChaosMaxLatency > 0 && options.ChaosMinLatency < options.ChaosMaxLatency {
				latency := time.Duration(rand.Int63n(int64(options.ChaosMaxLatency-options.
					ChaosMinLatency))) + options.ChaosMinLatency
				log.Printf("[CHAOS] Adding latency: %v\n", latency)
				time.Sleep(latency)
			}

			// Random error response
			if rand.Float64() < options.ChaosErrorRate {
				statusCodes := []int{500, 503, 502}
				errorCode := statusCodes[rand.Intn(len(statusCodes))]
				log.Printf("[CHAOS] Returning error: %d\n", errorCode)
				http.Error(w, http.StatusText(errorCode), errorCode)
				return
			}

			// Random throttling
			if rand.Float64() < options.ChaosThrottleRate {
				log.Printf("[CHAOS] Simulating throttling (429 Too Many Requests)\n")
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			// Random panic (gracefully recovered)
			if rand.Float64() < options.ChaosPanicRate {
				log.Printf("[CHAOS] Simulating panic\n")
				defer func() {
					if err := recover(); err != nil {
						log.Printf("[CHAOS] Recovered from panic: %v\n", err)
					}
				}()
				panic("Simulated Chaos Mode Panic")
			}

			// Proceed with normal handler
			next.ServeHTTP(w, r)
		}
	}
}

// TraceMiddleware returns a middleware function that adds trace IDs to requests.
// Generates unique trace IDs for request tracking and distributed tracing.
func TraceMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("TraceMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		traceID := generateTraceID()
		ctx := context.WithValue(r.Context(), traceIDKey, traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// trailingSlashMiddleware MiddlewareFunc redirects requests without a trailing slash to the same URL with a trailing slash.
// TODO: check if this  has become obsolete as the http handler is taking care.
func trailingSlashMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("trailingSlashMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" && !strings.HasSuffix(path, "/") {
			// redirect to the same URL with a trailing slash
			http.Redirect(w, r, path+"/", http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func generateTraceID() string {
	counter := requestCounter.Add(1)
	return fmt.Sprintf("%d-%d", counter, time.Now().UnixNano())
}

var requestCounter atomic.Int64

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