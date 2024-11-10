// Copyright 2024 by Oliver Sauer
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

// Simple HTTP Server with middleware and various option to handle requests and responses.

package hyperserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// logger is a global logger for the appServer. Use NewServer() to create a new appServer with a custom logger.
var logger = slog.Default()

func init() {
	logger.Info("Server initializing...")
}

// Environment management variable names
const (
	paramServerAddr = "SERVER_ADDR"
	paramHealthAddr = "HEALTH_ADDR"
	paramFileName   = "config.json"
)

var (
	isReady atomic.Bool
	isLive  atomic.Bool

	// Server metrics
	totalRequests     atomic.Uint64
	totalResponseTime atomic.Int64
	serverStart       time.Time

	clientLimiters = sync.Map{}
)

// RateLimit limits requests per second that can be requested from the appServer. Requires to add [RateLimitMiddleware]
type rateLimit = rate.Limit

// Config is a representation of the Server settings
type Config struct {
	Addr            string        `json:"addr"`
	HealthAddr      string        `json:"health-addr,omitempty"`
	RateLimit       rateLimit     `json:"rate-limit,omitempty"`
	Burst           int           `json:"burst,omitempty"`
	ReadTimeout     time.Duration `json:"read-timeout,omitempty"`
	WriteTimeout    time.Duration `json:"write-timeout,omitempty"`
	IdleTimeout     time.Duration `json:"idle-timeout,omitempty"`
	StaticDir       string        `json:"static-dir,omitempty"`
	TemplateDir     string        `json:"template-dir,omitempty"`
	RunHealthServer bool          `json:"run-health-server,omitempty"`
}

var defaultConfig = &Config{
	Addr:            ":8080",
	HealthAddr:      ":9080",
	RateLimit:       1,
	Burst:           10,
	ReadTimeout:     5 * time.Second,
	WriteTimeout:    10 * time.Second,
	IdleTimeout:     120 * time.Second,
	StaticDir:       "static/",
	TemplateDir:     "template/",
	RunHealthServer: false,
}

// NewConfig creates a new configuration for the server with a priority order. Environment variables override config file.
// 1. Environment variables
// 2. Config file (JSON)
// 3. Default values
func NewConfig() *Config {
	config := applyEnvVars(applyConfigFile(defaultConfig))
	return config
}

func applyEnvVars(config *Config) *Config {
	if addr := os.Getenv(paramServerAddr); addr != "" {
		config.Addr = addr
		logger.Info("Server address set from environment variable", "variable", paramServerAddr, "addr", addr)
	}
	if healthAddr := os.Getenv(paramHealthAddr); healthAddr != "" {
		config.HealthAddr = healthAddr
		logger.Info("Health endpoint address set from environment variable", "variable", paramHealthAddr, "addr", healthAddr)
	}
	return config
}

func applyConfigFile(config *Config) *Config {
	file, err := os.Open(paramFileName)
	if err != nil {
		logger.Warn("Failed to open config file", "error", err, "file-name", paramFileName)
		return config
	}

	// make sure file is closed after reading
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logger.Error("Failed to close file", "error", err, "file-name", file.Name())
		}
	}(file)

	decoder := json.NewDecoder(file)
	fileConfig := &Config{}
	if err := decoder.Decode(fileConfig); err != nil {
		logger.Info("Loading from config failed; Using environment and defaults")
		return config
	}
	logger.Info("Server configuration loaded from file", "file", paramFileName)
	mergeConfig(config, fileConfig)
	return config
}

// mergeConfig overrides default config with values of override if set
func mergeConfig(base *Config, override *Config) {
	if override.Addr != "" {
		base.Addr = override.Addr
	}
	if override.HealthAddr != "" {
		base.HealthAddr = override.HealthAddr
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
	if override.StaticDir != "" {
		base.StaticDir = override.StaticDir
	}
	if override.TemplateDir != "" {
		base.TemplateDir = override.TemplateDir
	}
}

// Server represents an HTTP appServer that can handle requests and responses.
type Server struct {
	mux          *http.ServeMux
	healthMux    *http.ServeMux
	appServer    *http.Server
	healthServer *http.Server
	config       *Config
	middleware   []middleware
}

// NewServer creates a new instance of the Server.
func NewServer(opts ...ServerOption) (*Server, error) {

	// init new appServer
	srv := &Server{
		mux:    http.NewServeMux(),
		config: NewConfig(),
	}

	// apply appServer options
	for _, opt := range opts {
		opt(srv)
	}

	// initialize the underlying application http appServer for serving requests
	srv.appServer = &http.Server{
		Addr:         srv.config.Addr,
		Handler:      srv.mux,
		ReadTimeout:  srv.config.ReadTimeout,
		WriteTimeout: srv.config.WriteTimeout,
		IdleTimeout:  srv.config.IdleTimeout,
	}

	srv.appServer.RegisterOnShutdown(srv.Shutdown)

	return srv, nil
}

func (srv *Server) Shutdown() {
	tp := uint64(0)
	resp := totalResponseTime.Load()
	if resp != 0 {
		tp = totalRequests.Load() / uint64(resp)
	}
	upTime := time.Since(serverStart)
	logger.Info("Server is shut down.", "up-time", upTime, "µs-in-handlers", resp, "total-req",
		totalRequests.Load(),
		"avg-handles-per-µs", tp)
}

// Run starts the appServer and listens for incoming requests.
func (srv *Server) Run() {
	// log appServer start time for collection up-time metric
	serverStart = time.Now()

	// Middleware chain ensuring MetricsMiddleware is run first
	srv.appServer.Handler = chainMiddleware(
		srv.MetricsMiddleware(srv.mux),
		srv.middleware...)

	if srv.config.RunHealthServer {
		srv.initHealthServer()
	}

	go srv.start()
	// create a channel to signal when shutdown is done
	done := make(chan struct{})
	// wait for OS signals to stop
	go srv.stop(done)

	// block until graceful shutdown happened and is completed
	<-done
	logger.Info("done")
	isLive.Store(false)
}

// helper function to initialise the health server
func (srv *Server) initHealthServer() {
	// initialize a lightweight http  for health endpoints listening on different port
	srv.healthMux = http.NewServeMux()
	srv.healthServer = &http.Server{
		Addr:    srv.config.HealthAddr,
		Handler: srv.healthMux,
	}
	logger.Info("Health server initialised.", "addr", srv.config.HealthAddr)

	// add built-in probing endpoints
	srv.healthMux.HandleFunc("/healthz/", srv.healthzHandler)
	srv.healthMux.HandleFunc("/readyz/", srv.readyzHandler)
	srv.healthMux.HandleFunc("/livez/", srv.livezHandler)
}

// start starts the appServer in a go-routine and serves incoming requests
func (srv *Server) start() {
	isReady.Store(true)
	isLive.Store(true)

	logger.Info("Server started.", "addr", srv.config.Addr)
	err := srv.appServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Failed to start appServer", "error", err)
		os.Exit(1)
	}
	logger.Info("appServer stopped listening")
}

// stop implements a graceful shutdown
func (srv *Server) stop(done chan struct{}) {

	// listen for OS signals to shut down the appServer.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	// block until a signal is received

	<-stop
	isReady.Store(false)
	logger.Info("received signal to shutdown appServer. Stopping...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.appServer.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown.", "error", err)
	}
	close(done)
}

// Handle registers the handler function for the given pattern.
// Example usage:
//
//	srv.Handle("/static", http.FileServer(http.Dir("./static")))
func (srv *Server) Handle(pattern string, handlerFunc http.HandlerFunc) {
	srv.mux.Handle(pattern, handlerFunc)
}

// HandleFunc registers the handler function for the given pattern.
// Example usage:
//
//	srv.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
//	    fmt.Fprintln(w, "Hello, world!")
//	})
func (srv *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	srv.mux.HandleFunc(pattern, handler)
}

// HandleFuncDynamic registers a handler function for the given pattern that dynamically generates data for the template.
// Example usage:
//
//	srv.HandleFuncDynamic("/time", "time.html", func(r *http.Request) interface{} {
//	    return map[string]interface{}{
//	        "timestamp": time.Now().Format("2006-01-02 15:04:05"),
//	    }
//	})
func (srv *Server) HandleFuncDynamic(pattern, template string, dataFunc DataFunc) {
	srv.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		data := dataFunc(r)
		err := templates.ExecuteTemplate(w, template, data)
		if err != nil {
			http.Error(w, "Failed to load content", http.StatusInternalServerError)
			logger.Error("Failed to load content", "error", err)
			return
		}
	})
}

// Cache templates for efficiency, with lazy initialisation
var templates *template.Template = nil

func EnsureTrailingSlash(dir string) string {
	if dir != "" && !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return dir
}

func (srv *Server) HandleTemplate(pattern, t string, data interface{}) {

	templateDir := EnsureTrailingSlash(srv.config.TemplateDir)
	if templates == nil {
		// lazy initialisation of templates
		parseTemplates(templateDir, srv)
	}
	srv.mux.HandleFunc(pattern, templateHandler(t, data))
}

func parseTemplates(templateDir string, srv *Server) {
	// check if an absolute path is provided and provide a warning
	if filepath.IsAbs(templateDir) {
		logger.Warn("Absolute path provided for template directory", "dir", templateDir)
	}

	// check if the template directory exists to avoid panic
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		wd, _ := os.Getwd()
		ad, _ := filepath.Abs(templateDir)
		logger.Error("Template directory not found", "error", err, "working-dir", wd, "abs-path", ad)
		srv.Shutdown()
		os.Exit(1)
	}
	templates = template.Must(template.ParseGlob(templateDir + "*.html"))
	logger.Info("Templates parsed.", "pattern", templateDir+"*.html")
}

// ServerOption using the functional options pattern.
// Pass options to the [Server] constructor to configure the appServer.
//
// Example:
//
// srv, err := NewServer(
//
//	                WithAddr(":8080"),
//	                WithMetrics(),)
//
//		if err != nil {
//			 log.Fatal(err)
//		}
//
// srv.handle("/", handler)
// srv.Run()
type ServerOption func(srv *Server)

// DataFunc returns a data structure for the template.
type DataFunc func(r *http.Request) interface{}

// WithHealthServer enables the health server on a different port.
func WithHealthServer() ServerOption {
	return func(srv *Server) {
		srv.config.RunHealthServer = true
	}
}

// WithAddr is a configuration option for the server to define listener port
func WithAddr(addr string) ServerOption {
	return func(srv *Server) {
		// validate the address
		_, port, err := net.SplitHostPort(addr)
		if err != nil && port == "" {
			logger.Error("setting address option", "error", err)
			// if the address failed to set, we must exit (no fallback to default etc.)
			os.Exit(1)
		}
		srv.config.Addr = addr
	}
}

// WithLogger replaces the default with a custom logger.
func WithLogger(l *slog.Logger) ServerOption {
	return func(srv *Server) {
		logger = l
	}
}

// setTimeouts helper to apply only custom values or retain the server default
func (srv *Server) setTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) {
	if readTimeout != 0 {
		srv.config.ReadTimeout = readTimeout
		srv.appServer.ReadTimeout = readTimeout
	}
	if writeTimeout != 0 {
		srv.config.WriteTimeout = writeTimeout
		srv.appServer.WriteTimeout = writeTimeout
	}
	if idleTimeout != 0 {
		srv.config.IdleTimeout = idleTimeout
		srv.appServer.IdleTimeout = idleTimeout
	}
}

// WithTimeouts adds timeouts to the appServer.
func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) ServerOption {
	return func(srv *Server) {
		srv.setTimeouts(readTimeout, writeTimeout, idleTimeout)
	}
}

// WithRateLimit sets rate limiting parameters of the appServer.
func WithRateLimit(limit rateLimit, burst int) ServerOption {
	return func(srv *Server) {
		srv.config.RateLimit = limit
		srv.config.Burst = burst
	}
}

// WithTemplateDir sets the directory for the templates.
func WithTemplateDir(dir string) ServerOption {
	return func(srv *Server) {
		srv.config.TemplateDir = dir
	}
}

// HealthCheckHandler returns a 204 status code for health check
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// PanicHandler simulations a panic situation in a handler to test proper recovery. See
func PanicHandler(w http.ResponseWriter, r *http.Request) {
	panic("Intentional panic.")
}

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
			logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("unhealthy")); err != nil {
			logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	}
}

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

// Use adds middleware to the appServer
func (srv *Server) Use(middleware ...middleware) {
	logger.Info("adding middleware")
	srv.middleware = append(srv.middleware, middleware...)
}

// Middleware is a function that wraps a http.Handler interface and returns a new http.HandlerFunc.
type middleware func(http.Handler) http.HandlerFunc

func (srv *Server) MetricsMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		totalRequests.Add(1)
		start := time.Now()
		next.ServeHTTP(w, r)
		totalResponseTime.Add(time.Since(start).Microseconds())
	}

}

// RequireAuthMiddleware middleware checks for a valid bearer token in the Authorization header.
func (srv *Server) RequireAuthMiddleware(next http.Handler) http.HandlerFunc {
	// Todo: implement auth middleware

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

// RequestLoggerMiddleware middleware logs the request details. Use with caution as it slows down the appServer.
func (srv *Server) RequestLoggerMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// create a new logging response writer to capture status code and bytes written
		lrw := &loggingResponseWriter{w, http.StatusOK, 0}

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		traceID := r.Context().Value(traceIDKey)
		if traceID == nil {
			traceID = ""
		}

		/*
			logger := logger.With(
				"from", ip,
				"method", r.Method,
				"url", r.URL.String(),
				"trace_id", traceID)
			logger.Info("Request received.")
		*/
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

// ResponseTimeMiddleware middleware logs the duration of the request.
func (srv *Server) ResponseTimeMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		logger.Info("Request duration", "duration", duration)
	}
}

// Consolidate error responses to maintain a consistent format.
func writeErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	response := map[string]string{"error": message}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		logger.Error("Failed to write error response", "error", err)
	}
}

// RateLimitMiddleware enforces a rate limit per-client.
func (srv *Server) RateLimitMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		limiterInterface, _ := clientLimiters.LoadOrStore(ip, rate.NewLimiter(srv.config.RateLimit, srv.config.Burst))
		limiter := limiterInterface.(*rate.Limiter)
		if !limiter.Allow() {
			writeErrorResponse(w, http.StatusTooManyRequests, "Too many requests")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// RecoveryMiddleware middleware recovers from panics and returns a 500 status code.
func (srv *Server) RecoveryMiddleware(next http.Handler) http.HandlerFunc {
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

func (srv *Server) HeadersMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// todo implement hardened mode
		hardened := false
		if !hardened {
			w.Header().Set("Server", "hyperserve")
		}

		for _, h := range securityHeaders {
			w.Header().Set(h.key, h.value)
		}

		// CORS headers
		if hardened {
			// Allow only requests from a specific origin
			w.Header().Set("Access-Control-Allow-Origin", "https://client-site.com")

		}
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

// templateHandler serves HTML templates with dynamic content.
func templateHandler(templateName string, data interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err := templates.ExecuteTemplate(w, templateName, data)
		if err != nil {
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
			slog.Error("Error rendering template", "error", err)
		}
	}
}

// Middleware definitions

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

// chainMiddleware helper to  apply multiple middlewares to a handler
func chainMiddleware(handler http.Handler, middlewares ...middleware) http.Handler {
	// reverse order to run first middleware passed first
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

var requestCounter atomic.Int64

func generateTraceID() string {
	counter := requestCounter.Add(1)
	return fmt.Sprintf("%d-%d", counter, time.Now().UnixNano())
}
