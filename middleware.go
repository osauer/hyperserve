package hyperserve

import (
	"context"
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

// MiddlewareFunc wraps a http.Handler interface and returns a new http.HandlerFunc.
type MiddlewareFunc func(http.Handler) http.HandlerFunc

// MiddlewareStack is a pre-defined collection of middleware that can be applied to a http.Handler.
type MiddlewareStack []MiddlewareFunc

// GlobalMiddlewareRoute is specifier that a MiddlewareStack applies to to all routes.
const GlobalMiddlewareRoute = "*"

// MiddlewareRegistry is a collection of MiddlewareStacks for different routes.
type MiddlewareRegistry struct {
	middleware map[string]MiddlewareStack
	exclude    []MiddlewareFunc
}

// NewMiddlewareRegistry creates a new MiddlewareRegistry. If globalMiddleware is not nil,
// it will be added to the registry as the default middleware applied to all routes.
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

// Add adds a MiddlewareStack to the MiddlewareRegistry for a given route.
func (mwr *MiddlewareRegistry) Add(route string, middleware MiddlewareStack) {
	mwr.middleware[route] = middleware
}

// Get returns the MiddlewareStack for a given route. If no middleware is found, it returns an empty MiddlewareStack.
func (mwr *MiddlewareRegistry) Get(route string) MiddlewareStack {
	ret := mwr.middleware[route]
	if ret == nil {
		logger.Warn("No middleware found for route", "route", route)
		ret = MiddlewareStack{}
	}
	return ret
}

// RemoveStack removes the MiddlewareStack for a given route. If no middleware is found, it does nothing.
func (mwr *MiddlewareRegistry) RemoveStack(route string) {
	delete(mwr.middleware, route)
}

// DefaultMiddleware is a predefined MiddlewareStack for basic server functionality. It will always be applied unless
// overridden with explicit options exclusion via Server.WithoutOption.
func DefaultMiddleware(server *Server) MiddlewareStack {
	return MiddlewareStack{
		MetricsMiddleware(server),
		RequestLoggerMiddleware,
		RecoveryMiddleware}
}

// SecureAPI is a predefined MiddlewareStack for secure API endpoints.
func SecureAPI(options ServerOptions) MiddlewareStack {
	return MiddlewareStack{
		AuthMiddleware,
		RateLimitMiddleware(options)}
}

// SecureWeb is a predefined MiddlewareStack for secure web endpoints.
func SecureWeb(options ServerOptions) MiddlewareStack {
	return MiddlewareStack{HeadersMiddleware(options)}
}

// FileServer is a predefined MiddlewareStack for serving static files.
func FileServer(options ServerOptions) MiddlewareStack {
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

type Header struct {
	key   string
	value string
}

// MetricsMiddleware MiddlewareFunc collects metrics for requests and response times.
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

// AuthMiddleware MiddlewareFunc checks for a valid bearer token in the Authorization header.
func AuthMiddleware(next http.Handler) http.HandlerFunc {
	// Todo: implement auth MiddlewareFunc
	logger.Info("AuthMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for auth token
		authHeader := r.Header.Get(authorizationHeader)

		// check if header has bearer token
		if !strings.HasPrefix(authHeader, bearerTokenPrefix) {
			http.Error(w, "Unauthorized: Bearer token required", http.StatusUnauthorized)
			return
		}
		sessionId := strings.TrimPrefix(authHeader, bearerTokenPrefix)
		if sessionId == "" {
			http.Error(w, "Unauthorized: Bearer token invalid", http.StatusUnauthorized)
			return
		}

		// add session and ID to the context
		ctx := context.WithValue(r.Context(), sessionIDKey, sessionId)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// RequestLoggerMiddleware MiddlewareFunc logs the request details. Use with caution as it slows down the httpServer.
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

// ResponseTimeMiddleware MiddlewareFunc logs the duration of the request. Can be used without the RequestLoggerMiddleware.
func ResponseTimeMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("ResponseTimeMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		logger.Info("Request duration", "duration", duration)
	}
}

// RecoveryMiddleware MiddlewareFunc recovers from panics and returns a 500 status code.
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

// RateLimitMiddleware enforces a rate limit per-client ip address
func RateLimitMiddleware(options ServerOptions) MiddlewareFunc {
	logger.Info("RateLimitMiddleware enabled")
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			limiterInterface, _ := clientLimiters.LoadOrStore(ip, rate.NewLimiter(options.RateLimit, options.Burst))
			limiter := limiterInterface.(*rate.Limiter)
			if limiter.Allow() {
				// todo: add a header to the response to indicate the rate limit, left tokens etc.
				next.ServeHTTP(w, r)
			} else {
				// todo be gentle with the response and provide a retry-after header
				writeErrorResponse(w, http.StatusTooManyRequests, "Rate limit exceeded")
			}
			return
		}
	}
}

// securityHeaders provide headers for HeadersMiddleware
var securityHeaders = []Header{
	{"X-Content-Type-Options", "nosniff"},                                // Prevent MIME-type sniffing
	{"X-Frame-Options", "DENY"},                                          // Mitigate clickjacking
	{"X-XSS-Protection", "1; mode=block"},                                // Enable XSS protection in browsers
	{"Strict-Transport-Security", "max-age=63072000; includeSubDomains"}, // Enforce HTTPS (if applicable)
	{"Referrer-Policy", "no-referrer"},                                   // Reduce referrer leakage
	{"Feature-Policy", "geolocation 'none'; midi 'none'; notifications 'none'; push 'none'; sync-xhr 'none'; microphone 'none'; camera 'none'; magnetometer 'none'; gyroscope 'none'; speaker 'none'; vibrate 'none'; fullscreen 'self'; payment 'none';"},
	{"Expect-CT", "max-age=86400, enforce, report-uri='https://example.com/report-ct'"}, // Expect Certificate Transparency
	{"Content-Security-Policy", "default-src 'self'"},                                   // Control resources the client is allowed to load
	{"Access-Control-Allow-Methods", "GET, POST, OPTIONS"},                              // Allowed methods
	{"Access-Control-Allow-Headers", "Content-Type, Authorization"},                     // Allowed headers
	{"Access-Control-Allow-Credentials", "true"},                                        // If cookies or credentials are needed
	{"Access-Control-Max-Age", "600"},                                                   // Pre-flight request cache
}

// HeadersMiddleware MiddlewareFunc adds security headers to the response.
func HeadersMiddleware(options ServerOptions) MiddlewareFunc {
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

// ChaosMiddleware applies random disturbances to simulate chaos
func ChaosMiddleware(next http.Handler, options ServerOptions) http.Handler {
	logger.Info("ChaosMiddleware enabled")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})
}

// TraceMiddleware MiddlewareFunc
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