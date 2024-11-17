package hyperserve

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// MiddlewareFunc wraps a http.Handler interface and returns a new http.HandlerFunc.
type MiddlewareFunc func(http.Handler) http.HandlerFunc

// MiddlewareStack is a pre-defined collection of middlewares that can be applied to a http.Handler.
type MiddlewareStack []MiddlewareFunc

// DefaultMiddleware is a predefined MiddlewareStack for basic server functionality. It will always be applied unless
// overridden with explicit options exclusion via Server.WithoutOption.
func DefaultMiddleware() MiddlewareStack {
	return MiddlewareStack{
		MetricsMiddleware,
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
	return MiddlewareStack{HeadersMiddleware}
}

// FileServer is a predefined MiddlewareStack for serving static files.
func FileServer() MiddlewareStack {
	return MiddlewareStack{HeadersMiddleware}
}

// Middleware definitions

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

// securityHeaders provide headers for SecurityHeadersMiddleware MiddlewareFunc
var securityHeaders = []Header{
	// Prevent MIME-type sniffing
	{"X-Content-Type-Options", "nosniff"},
	// Mitigate clickjacking
	{"X-Frame-Options", "DENY"},
	// Enable XSS protection in browsers
	{"X-XSS-Protection", "1; mode=block"},
	// Enforce HTTPS (if applicable)
	{"Strict-Transport-Security", "max-age=63072000; includeSubDomains"},
	// Control resources the client is allowed to load
	{"Content-Security-Policy", "default-src 'self'"},
}

// MetricsMiddleware MiddlewareFunc collects metrics for requests and response times.
func MetricsMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("MetricsMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		totalRequests.Add(1)
		start := time.Now()
		next.ServeHTTP(w, r)
		totalResponseTime.Add(time.Since(start).Microseconds())
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

// RequestLoggerMiddleware MiddlewareFunc logs the request details. Use with caution as it slows down the appServer.
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

func HeadersMiddleware(next http.Handler) http.HandlerFunc {
	logger.Info("HeadersMiddleware enabled")
	return func(w http.ResponseWriter, r *http.Request) {
		// todo implement hardened mode
		hardened := false
		if !hardened {
			w.Header().Set("Server", "hyperserve")
		}

		for _, h := range securityHeaders {
			w.Header().Set(h.key, h.value)
		}

		// ToDo add allowed site origin(s) to the header
		// Allow only requests from a specific origin
		w.Header().Set("Access-Control-Allow-Origin", "https://client-site.com")

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true") // If cookies or credentials are needed

		// Handle preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// call the next handler if not in preflight
		next.ServeHTTP(w, r)
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

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bytesWritten += n
	return n, err
}
