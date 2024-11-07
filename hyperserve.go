// Description: A simple HTTP server that can handle requests and responses.
// The server can be configured with middleware functions to add functionality to the handlers.
// The server can be rate limited to a certain number of requests per second.

package hyperserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// default values for the server
const (
	defaultRateLimit RateLimit = 1  // default requests epr second
	defaultBurst     int       = 10 // default maximum number of tokens that can be served

	// http listener defaults
	defaultReadTimeout  = 5 * time.Second
	defaultWriteTimeout = 10 * time.Second
	defaultIdleTimeout  = 120 * time.Second // allows sessions to remain idle
	defaultAddr         = ":8080"
	// Environment management variable names
	configServerAddr = "SERVER_ADDR"
	configFileName   = "config.json"
)

// RateLimit limits requests per second that can be requested from the server
type RateLimit = rate.Limit

// Config is a representation of the server settings
type Config struct {
	Addr         string        `json:"addr"`
	RateLimit    RateLimit     `json:"rate-limit"`
	Burst        int           `json:"burst"`
	ReadTimeout  time.Duration `json:"readTimeout"`
	WriteTimeout time.Duration `json:"writeTimeout"`
	IdleTimeout  time.Duration `json:"idleTimeout"`
}

// NewConfig creates a new config with a priority order:
// 1. Environment variables
// 2. Config file (JSON)
// 3. Default values
func NewConfig() *Config {
	config := &Config{
		Addr:         defaultAddr,
		RateLimit:    defaultRateLimit,
		Burst:        defaultBurst,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		IdleTimeout:  defaultIdleTimeout,
	}
	// step 1 load from environment variables
	if addr := os.Getenv(configServerAddr); addr != "" {
		config.Addr = addr
	}
	// step 2 load from config file if available
	if fileConfig, err := loadConfigFromFile(configFileName); err == nil {
		mergeConfig(config, fileConfig)
	} else {
		slog.Info("No config file found")
	}
	return config
}

// mergeConfig overrides default config with values of override if set
func mergeConfig(base *Config, override *Config) {
	if override.Addr != "" {
		base.Addr = override.Addr
	}
	if override.RateLimit != 0 {
		base.RateLimit = override.RateLimit
	}
	if override.Burst != 0 {
		base.Burst = override.Burst
	}
	if override.ReadTimeout != 0 {
		base.ReadTimeout = override.ReadTimeout
	}
	if override.WriteTimeout != 0 {
		base.WriteTimeout = override.WriteTimeout
	}
	if override.IdleTimeout != 0 {
		base.IdleTimeout = override.IdleTimeout
	}
}

// loadConfigFromFile loads a configuration from filename and returns a configuration on success or an error otherwise.
func loadConfigFromFile(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := &Config{}
	if err := decoder.Decode(config); err != nil {
		return nil, err
	}
	return config, nil
}

var (
	isReady atomic.Bool
	isLive  atomic.Bool

	// Server metrics
	totalRequests     atomic.Uint64
	totalResponseTime atomic.Int64
	serverStart       time.Time

	clientLimiters = sync.Map{}
)

// Middleware is a function that wraps a http.Handler interface and returns a new http.HandlerFunc.
type middleware func(http.Handler) http.HandlerFunc

// ServerOption using the functional options pattern. Pass options to the server constructor to configure the server.
type ServerOption func(srv *Server)

// Server represents an HTTP server that can handle requests and responses.
type Server struct {
	mux        *http.ServeMux
	server     *http.Server
	logger     *slog.Logger
	config     *Config
	middleware []middleware
}

// NewAPIServer creates a new instance of the Server.
func NewAPIServer(opts ...ServerOption) (*Server, error) {

	// init new server
	srv := &Server{
		mux:    http.NewServeMux(),
		logger: slog.Default(), // default logger
		config: NewConfig(),
	}

	// apply server options
	for _, opt := range opts {
		opt(srv)
	}

	// initialize the underlying http server
	srv.server = &http.Server{
		Addr:         srv.config.Addr,
		Handler:      srv.mux,
		ReadTimeout:  srv.config.ReadTimeout,
		WriteTimeout: srv.config.WriteTimeout,
		IdleTimeout:  srv.config.IdleTimeout,
	}

	srv.server.RegisterOnShutdown(srv.Shutdown)

	return srv, nil
}

func (srv *Server) Shutdown() {
	tp := uint64(0)
	resp := totalResponseTime.Load()
	if resp != 0 {
		tp = totalRequests.Load() / uint64(resp)
	}
	upTime := time.Since(serverStart)
	srv.logger.Info("Server is shut down.", "up-time", upTime, "µs-in-handlers", resp, "total-req",
		totalRequests.Load(),
		"avg-handles-per-µs", tp)
}

func (srv *Server) MetricsMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO totalRequests are not actual requests, but requests hitting the handler successfully
		// TODO totalResposne Time needs to reflect this ,it's totalTimeInHandler
		// TODO find way to measure acual response times, including pattern matching etc.
		totalRequests.Add(1)
		start := time.Now()
		next.ServeHTTP(w, r)
		totalResponseTime.Add(time.Since(start).Microseconds())
	}

}

// Run starts the server and listens for incoming requests.
func (srv *Server) Run() {
	// log server start time for collection up-time metric
	serverStart = time.Now()
	// apply middleware to the server
	if len(srv.middleware) > 0 {
		srv.server.Handler = ChainMiddleware(srv.mux, srv.middleware...)
	}
	// force MetricsMiddleware to be present and loaded first
	srv.server.Handler = srv.MetricsMiddleware(srv.server.Handler)

	// add built-in probing endpoints
	srv.Handle("/healthz/", srv.healthzHandler)
	srv.Handle("/readyz/", srv.readyzHandler)
	srv.Handle("/livez/", srv.livezHandler)

	go srv.start()
	// create a channel to signal when shutdown is done
	done := make(chan struct{})
	// wait for OS signals to stop
	go srv.stop(done)

	// block until graceful shutdown happened and is completed
	<-done
	srv.logger.Info("done")
	isLive.Store(false)
}

// start starts the server in a go-routine and serves incoming requests
func (srv *Server) start() {
	isReady.Store(true)
	isLive.Store(true)

	srv.logger.Info("Server started.", "addr", srv.config.Addr)
	err := srv.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		srv.logger.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped listening")
}

// stop implements a graceful shutdown
func (srv *Server) stop(done chan struct{}) {

	// listen for OS signals to shutdown the server.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	// block until a signal is received

	<-stop
	isReady.Store(false)
	srv.logger.Info("received signal to shutdown server. Stopping...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.server.Shutdown(ctx); err != nil {
		srv.logger.Error("Server forced to shutdown.", "error", err)
	}
	close(done)
}

// Handle registers a handler for the given pattern.
func (srv *Server) Handle(pattern string, handler http.HandlerFunc) {
	srv.mux.HandleFunc(pattern, handler)
}

// WithAddr adds a listener port to the server. Overwrites existing configuration when applied.
func WithAddr(addr string) ServerOption {
	return func(srv *Server) {
		// validate the address
		_, port, err := net.SplitHostPort(addr)
		if err != nil && port == "" {
			srv.logger.Error("setting address option", "error", err)
			// if the address failed to set, we must exit (no fallback to default etc.)
			os.Exit(1)
		}
		srv.config.Addr = addr
	}
}

// WithLogger adds a structured logger to the server.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(srv *Server) {
		srv.logger = logger
	}
}

// WithTimeouts adds  timeouts to the server.
func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) ServerOption {
	return func(srv *Server) {
		if srv.server == nil {
			panic("server is nil")
		}
		// TODO this needs to set the srv.config values, plus the server values. Setter?
		srv.server.ReadTimeout = readTimeout
		srv.server.WriteTimeout = writeTimeout
		srv.server.IdleTimeout = idleTimeout
	}
}

// WithRateLimit sets rate limiting parameters of the server.
func WithRateLimit(limit RateLimit, burst int) ServerOption {
	return func(srv *Server) {
		srv.config.RateLimit = limit
		srv.config.Burst = burst
	}
}

// Use adds middleware to the server
func (srv *Server) Use(middleware ...middleware) {
	srv.logger.Info("adding middleware")
	srv.middleware = append(srv.middleware, middleware...)
}

func (srv *Server) userDataHandler(w http.ResponseWriter, r *http.Request) {
	// get the user ID from the path
	userID, err := strconv.Atoi(r.PathValue("userID"))

	response := fmt.Sprintf("User Data: %d", userID)
	bw, err := w.Write([]byte(response))
	if err != nil {
		srv.logger.Error("Failed to w"+
			""+
			"rite response", "Error", err, "Bytes written", bw)
	}
}

// HandleHealthCheck returns a 204 status code for health check
func HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// HandlePanic simulations a panic situation in a handler to test proper recovery. See
func HandlePanic(w http.ResponseWriter, r *http.Request) {
	panic("Intentional panic.")
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (srv *Server) livezHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "alive", &isLive)
}

func (srv *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "ready", &isReady)
}

func (srv *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "ok", &isLive)
}

func (srv *Server) healthHandlerHelper(w http.ResponseWriter, request *http.Request, probe string,
	status *atomic.Bool) {
	if status.Load() {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(probe)); err != nil {
			srv.logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("unhealthy")); err != nil {
			srv.logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	}
}

// Header context keys
type contextKey string

const (
	authorizationHeader            = "Authorization"
	bearerTokenPrefix              = "Bearer "
	sessionIDKey        contextKey = "sessionID"
	userIDKey           contextKey = "userID"
	traceIDKey          contextKey = "traceID"
	// Header constants
)

type Header struct {
	key   string
	value string
}

// securityHeaders provide headers for SecurityHeadersMiddleware middleware
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

// RequireAuthMiddleware middleware checks for a valid bearer token in the Authorization header.
func (srv *Server) RequireAuthMiddleware(next http.Handler) http.HandlerFunc {
	// Todo: implement auth middleware
	// Todo: implement error logging, enabling logger in the handler
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

		userID, ok := r.Context().Value(userIDKey).(int)
		if !ok {
			http.Error(w, "Unauthorized: invalid user ID", http.StatusUnauthorized)
			return
		}
		// add session and ID to the context
		ctx := context.WithValue(r.Context(), sessionIDKey, sessionId)
		ctx = context.WithValue(ctx, userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// RequestLoggerMiddleware middleware logs the request details. Use with caution as it slows down the server.
func (srv *Server) RequestLoggerMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// create a new logging response writer to capture status code and bytes written
		lrw := &loggingResponseWriter{w, http.StatusOK, 0}

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		traceID := r.Context().Value(traceIDKey)
		if traceID == nil {
			traceID = ""
		}

		logger := srv.logger.With(
			"from", ip,
			"method", r.Method,
			"url", r.URL.String(),
			"trace_id", traceID)
		logger.Info("Request received.")

		start := time.Now()
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)
		logger.Info("Request completed",
			"status", lrw.statusCode,
			"duration", duration)
	}
}

// ResponseTimeMiddleware middleware logs the duration of the request.
func (srv *Server) ResponseTimeMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		srv.logger.Info("Request duration", "duration", duration)
		next.ServeHTTP(w, r)
	}
}

// RateLimitMiddleware enforces a rate limit per-client.
func (srv *Server) RateLimitMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srv.logger.Info("in rate limiter")
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		limiterInterface, _ := clientLimiters.LoadOrStore(ip, rate.NewLimiter(srv.config.RateLimit, srv.config.Burst))
		limiter := limiterInterface.(*rate.Limiter)
		if !limiter.Allow() {
			srv.logger.Info("rate limit reached", "rate-limit", srv.config.RateLimit, "burst", srv.config.Burst)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		srv.logger.Debug("rate limit not yet reached.")
		next.ServeHTTP(w, r)
	}
}

// Recovery middleware recovers from panics and returns a 500 status code.
func (srv *Server) Recovery(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				srv.logger.Error("Panic recovered", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	}
}

// SecurityHeadersMiddleware middleware to help mitigate common security risks when handling content in browsers.
func (srv *Server) SecurityHeadersMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, h := range securityHeaders {
			w.Header().Set(h.key, h.value)
		}
		next.ServeHTTP(w, r)
	}
}

// TraceMiddleware middleware
func (srv *Server) TraceMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceID := generateTraceID()
		ctx := context.WithValue(r.Context(), traceIDKey, traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// trailingSlashMiddleware middleware redirects requests without a trailing slash to the same URL with a trailing slash.
// TODO: check if this  has become obsolete as the http handler is taking care.
func (srv *Server) trailingSlashMiddleware(next http.Handler) http.HandlerFunc {
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

// ChainMiddleware helper to  apply multiple middlewares to a handler
func ChainMiddleware(handler http.Handler, middlewares ...middleware) http.Handler {
	// reverse order to run first middleware passed first
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
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

var requestCounter atomic.Int64

func generateTraceID() string {
	counter := requestCounter.Add(1)
	return fmt.Sprintf("%d-%d", counter, time.Now().UnixNano())
}
