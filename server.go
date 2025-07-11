// Copyright 2024 by Oliver Sauer
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

// Simple HTTP Server with MiddlewareFunc and various option to handle requests and responses.

package hyperserve

import (
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"io"
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

// logger is a global logger for the httpServer. Use NewServer() to create a new httpServer with a custom logger.
var logger = slog.Default()

func init() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	logger.Info("Server initializing...")
}

// Environment management variable names
const (
	paramServerAddr         = "SERVER_ADDR"
	paramHealthAddr         = "HEALTH_ADDR"
	paramHardenedMode       = "HS_HARDENED_MODE"
	paramFileName           = "options.json"
	paramMCPEnabled         = "HS_MCP_ENABLED"
	paramMCPEndpoint        = "HS_MCP_ENDPOINT"
	paramMCPServerName      = "HS_MCP_SERVER_NAME"
	paramMCPServerVersion   = "HS_MCP_SERVER_VERSION"
	paramMCPFileToolRoot    = "HS_MCP_FILE_TOOL_ROOT"
)

// rateLimit limits requests per second that can be requested from the httpServer. Requires to add [RateLimitMiddleware]
type rateLimit = rate.Limit

// rateLimiterEntry stores a rate limiter with last access time for cleanup
type rateLimiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// Server represents an HTTP server that can handle requests and responses.
// It provides middleware support, health checks, template rendering, and various configuration options.
type Server struct {
	mux               *http.ServeMux
	healthMux         *http.ServeMux
	httpServer        *http.Server
	healthServer      *http.Server
	middleware        *MiddlewareRegistry
	templates         *template.Template
	templatesMu       sync.Mutex
	Options           *ServerOptions
	isReady           atomic.Bool
	isRunning         atomic.Bool
	totalRequests     atomic.Uint64
	totalResponseTime atomic.Int64
	serverStart       time.Time
	clientLimiters    map[string]*rateLimiterEntry
	limitersMu        sync.RWMutex
	cleanupTicker     *time.Ticker
	cleanupDone       chan bool
	staticRoot        *os.Root
	templateRoot      *os.Root
	mcpHandler        *MCPHandler
}

// NewServer creates a new instance of the Server with the given options.
// It initializes the server with default middleware and applies all provided ServerOptionFunc options.
// Returns an error if any of the options fail to apply.
func NewServer(opts ...ServerOptionFunc) (*Server, error) {
	// init new httpServer
	srv := &Server{
		mux:            http.NewServeMux(),
		Options:        NewServerOptions(),
		templates:      nil,
		templatesMu:    sync.Mutex{},
		clientLimiters: make(map[string]*rateLimiterEntry),
		cleanupDone:    make(chan bool),
	}
	srv.middleware = NewMiddlewareRegistry(DefaultMiddleware(srv))
	logger.Info("Default middleware registered", "middlewares", []string{"MetricsMiddleware", "RequestLoggerMiddleware", "RecoveryMiddleware"})

	// apply httpServer options
	for _, opt := range opts {
		if err := opt(srv); err != nil {
			return nil, err
		}
	}

	// Static root will be initialized lazily when HandleStatic is called

	if srv.Options.TemplateDir != "" {
		templateRoot, err := os.OpenRoot(srv.Options.TemplateDir)
		if err != nil {
			logger.Warn("Failed to open template root directory", "error", err, "dir", srv.Options.TemplateDir)
		} else {
			srv.templateRoot = templateRoot
			logger.Info("Template root initialized", "dir", srv.Options.TemplateDir)
		}
	}

	// Initialize MCP handler if enabled
	if srv.Options.MCPEnabled {
		serverInfo := MCPServerInfo{
			Name:    srv.Options.MCPServerName,
			Version: srv.Options.MCPServerVersion,
		}
		srv.mcpHandler = NewMCPHandler(serverInfo)
		
		// Register built-in tools if enabled
		if srv.Options.MCPToolsEnabled {
			// File tools
			fileReadTool, err := NewFileReadTool(srv.Options.MCPFileToolRoot)
			if err != nil {
				logger.Warn("Failed to create file read tool", "error", err)
			} else {
				srv.mcpHandler.RegisterTool(fileReadTool)
			}
			
			listDirTool, err := NewListDirectoryTool(srv.Options.MCPFileToolRoot)
			if err != nil {
				logger.Warn("Failed to create list directory tool", "error", err)
			} else {
				srv.mcpHandler.RegisterTool(listDirTool)
			}
			
			// HTTP and calculator tools
			srv.mcpHandler.RegisterTool(NewHTTPRequestTool())
			srv.mcpHandler.RegisterTool(NewCalculatorTool())
		}
		
		// Register built-in resources if enabled
		if srv.Options.MCPResourcesEnabled {
			srv.mcpHandler.RegisterResource(NewConfigResource(srv.Options))
			srv.mcpHandler.RegisterResource(NewMetricsResource(srv))
			srv.mcpHandler.RegisterResource(NewSystemResource())
			srv.mcpHandler.RegisterResource(NewLogResource(srv.Options.MCPLogResourceSize))
		}
		
		// Register MCP endpoint
		srv.mux.Handle(srv.Options.MCPEndpoint, srv.mcpHandler)
		logger.Info("MCP handler initialized", "endpoint", srv.Options.MCPEndpoint)
	}

	// Start cleanup ticker for rate limiters (run every 5 minutes)
	srv.cleanupTicker = time.NewTicker(5 * time.Minute)
	go srv.cleanupRateLimiters()

	srv.isReady.Store(true)
	return srv, nil
}

// Run starts the server and listens for incoming requests.
// It sets up TLS if enabled, starts the health server if configured, and handles graceful shutdown.
// Returns an error if the server fails to start or encounters an error during operation.
func (srv *Server) Run() error {
	// log httpServer start time for collection up-time metric
	srv.serverStart = time.Now()
	srv.isRunning.Store(true)

	// Check if we're running in stdio mode for MCP
	if srv.Options.MCPEnabled && srv.Options.MCPTransport == StdioTransport {
		// Run MCP in stdio mode
		if srv.mcpHandler == nil {
			return fmt.Errorf("MCP handler not initialized for stdio transport")
		}
		return srv.mcpHandler.RunStdioLoop()
	}

	// initialize the underlying http httpServer for serving requests
	srv.httpServer = &http.Server{
		Handler:      srv.mux,
		ReadTimeout:  srv.Options.ReadTimeout,
		WriteTimeout: srv.Options.WriteTimeout,
		IdleTimeout:  srv.Options.IdleTimeout,
	}
	srv.httpServer.RegisterOnShutdown(srv.logServerMetrics)

	// apply available middleware to the httpServer
	srv.httpServer.Handler = srv.middleware.applyToMux(srv.mux)

	if srv.Options.RunHealthServer {
		err := srv.initHealthServer()
		if err != nil {
			return err
		}
	}

	// Channel for server errors
	serverErr := make(chan error, 1)

	// Run the server in a goroutine
	go func() {
		if srv.Options.EnableTLS {
			if srv.Options.CertFile == "" || srv.Options.KeyFile == "" {
				logger.Error("TLS enabled but no key or cert file provided.", "key", srv.Options.KeyFile,
					"cert", srv.Options.CertFile)
				return
			}
			// Configure TLS settings
			srv.httpServer.TLSConfig = srv.tlsConfig()
			srv.httpServer.Addr = srv.Options.TLSAddr
			logger.Info("Starting TLS server on", "addr", srv.Options.TLSAddr)
			serverErr <- srv.httpServer.ListenAndServeTLS(srv.Options.CertFile, srv.Options.KeyFile)
		} else {
			srv.httpServer.Addr = srv.Options.Addr
			logger.Info("Starting server on", "addr", srv.Options.Addr)
			serverErr <- srv.httpServer.ListenAndServe()
		}
	}()

	// Graceful shutdown handling
	return srv.handleShutdown(serverErr)
}

func (srv *Server) logServerMetrics() {
	tp := uint64(0)
	resp := srv.totalResponseTime.Load()
	if resp != 0 {
		tp = srv.totalRequests.Load() / uint64(resp)
	}
	upTime := time.Since(srv.serverStart)
	logger.Info("Server metrics:", "up-time", upTime, "µs-in-handlers", resp, "total-req",
		srv.totalRequests.Load(),
		"avg-handles-per-µs", tp)
}

func (srv *Server) tlsConfig() *tls.Config {
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	if srv.Options.FIPSMode {
		// FIPS 140-3 compliant cipher suites and curves only
		config.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_AES_128_GCM_SHA256, // TLS 1.3 FIPS approved
			tls.TLS_AES_256_GCM_SHA384, // TLS 1.3 FIPS approved
		}
		config.CurvePreferences = []tls.CurveID{
			tls.CurveP256,
			tls.CurveP384,
		}
		logger.Info("TLS configured in FIPS 140-3 mode")
	} else {
		// Standard cipher suites including post-quantum ready
		config.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_AES_128_GCM_SHA256,       // TLS 1.3 cipher suite
			tls.TLS_AES_256_GCM_SHA384,       // TLS 1.3 cipher suite
			tls.TLS_CHACHA20_POLY1305_SHA256, // TLS 1.3 cipher suite
		}
		// CurvePreferences nil enables post-quantum X25519MLKEM768 by default in Go 1.24
		config.CurvePreferences = nil
	}

	// Enable Encrypted Client Hello if configured
	if srv.Options.EnableECH && len(srv.Options.ECHKeys) > 0 {
		// ECH configuration will be automatically handled by Go 1.24's crypto/tls
		// when ECH keys are provided in the Config
		logger.Info("Encrypted Client Hello (ECH) enabled")
	}

	return config
}

// AddMiddleware adds a single middleware function to the specified route.
// Use "*" as the route to apply middleware globally to all routes.
func (srv *Server) AddMiddleware(route string, mw MiddlewareFunc) {
	srv.middleware.Add(route, MiddlewareStack{mw})
	logger.Debug("Middleware registered", "route", route, "count", 1)
}

// AddMiddlewareStack adds a collection of middleware functions to the specified route.
// The middleware stack is applied in the order provided.
func (srv *Server) AddMiddlewareStack(route string, mw MiddlewareStack) {
	srv.middleware.Add(route, mw)
	logger.Debug("Middleware stack registered", "route", route, "count", len(mw))
}

func (srv *Server) initHealthServer() error {
	// Initialize a lightweight HTTP server for health endpoints
	srv.healthMux = http.NewServeMux()
	srv.healthMux.HandleFunc("/healthz/", srv.healthzHandler)
	srv.healthMux.HandleFunc("/readyz/", srv.readyzHandler)
	srv.healthMux.HandleFunc("/livez/", srv.livezHandler)

	srv.healthServer = &http.Server{
		Addr:    srv.Options.HealthAddr,
		Handler: srv.healthMux,
		BaseContext: func(_ net.Listener) context.Context {
			return context.WithValue(context.Background(), "health", true)
		},
	}

	// Channel to receive errors from the health server goroutine
	healthErrChan := make(chan error, 1)

	go func() {
		logger.Info("Starting health server.", "addr", srv.Options.HealthAddr)
		if err := srv.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Health server encountered an error.", "error", err)
			healthErrChan <- err
		}
	}()

	// Optionally, wait for the server to start or fail
	select {
	case err := <-healthErrChan:
		return err
	case <-time.After(100 * time.Millisecond):
		// Assume server started successfully after a short delay
		return nil
	}
}

func (srv *Server) handleShutdown(serverErr chan error) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	select {
	case sig := <-quit:
		logger.Info("Shutting down server.", "reason", sig)
		srv.isReady.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.isRunning.Store(false)
		return srv.shutdown(ctx)
	case err := <-serverErr:
		return err
	}
}

func (srv *Server) shutdown(ctx context.Context) error {
	// Create an error channel to collect errors from goroutines
	errChan := make(chan error, 2)
	var wg sync.WaitGroup

	// Shutdown health server if it's running
	if srv.Options.RunHealthServer && srv.healthServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("Shutting down health server.")
			if err := srv.healthServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				logger.Error("Error during health server shutdown.", "error", err)
				errChan <- fmt.Errorf("health server shutdown error: %w", err)
			}
		}()
	}

	// Shutdown http server
	if srv.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("Shutting down http server.")
			if err := srv.httpServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				logger.Error("Error during main server shutdown.", "error", err)
				errChan <- fmt.Errorf("main server shutdown error: %w", err)
			}
		}()
	}

	// Wait for both shutdowns to complete
	wg.Wait()
	close(errChan)

	// Collect errors
	var shutdownErr error
	for err := range errChan {
		if shutdownErr == nil {
			shutdownErr = err
		} else {
			// Combine errors
			shutdownErr = fmt.Errorf("%v; %w", shutdownErr, err)
		}
	}

	// Clean up resources
	srv.stopCleanup()

	// Close os.Root handles if they exist
	if srv.staticRoot != nil {
		if err := srv.staticRoot.Close(); err != nil {
			logger.Error("Failed to close static root", "error", err)
		}
	}
	if srv.templateRoot != nil {
		if err := srv.templateRoot.Close(); err != nil {
			logger.Error("Failed to close template root", "error", err)
		}
	}

	return shutdownErr
}

// Stop gracefully stops the server with a default timeout of 10 seconds
func (srv *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.isReady.Store(false)
	srv.isRunning.Store(false)
	return srv.shutdown(ctx)
}

func (srv *Server) shutdownHealthServer(ctx context.Context) error {
	if srv.Options.RunHealthServer {
		logger.Info("Shutting down health server.")
		// Close any dependencies if needed
		// ...
		return srv.healthServer.Shutdown(ctx)
	} else {
		return nil
	}
}

func (srv *Server) WithOutStack(stack MiddlewareStack) error {
	if srv.isRunning.Load() {
		return fmt.Errorf("Cannot change middleware after httpServer has started.")
	}
	srv.middleware.exclude = append(srv.middleware.exclude, stack...)
	return nil
}

// Handle registers the handler function for the given pattern.
// This is a wrapper around http.ServeMux.Handle that integrates with the server's middleware system.
// Example usage:
//
//	srv.Handle("/static", http.FileServer(http.Dir("./static")))
func (srv *Server) Handle(pattern string, handlerFunc http.HandlerFunc) {
	srv.mux.Handle(pattern, handlerFunc)
}

// HandleFunc registers the handler function for the given pattern.
// This is a wrapper around http.ServeMux.HandleFunc that integrates with the server's middleware system.
// Example usage:
//
//	srv.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
//	    fmt.Fprintln(w, "Hello, world!")
//	})
func (srv *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	srv.mux.HandleFunc(pattern, handler)
}

// HandleFuncDynamic registers a handler that renders templates with dynamic data.
// The dataFunc is called for each request to generate the data passed to the template.
// Returns an error if template parsing fails.
func (srv *Server) HandleFuncDynamic(pattern, tmplName string, dataFunc DataFunc) error {
	if err := srv.parseTemplates(); err != nil {
		logger.Error("Failed to parse templates", "error", err)
		return err
	}
	
	// Check if the template exists
	if srv.templates != nil && srv.templates.Lookup(tmplName) == nil {
		return fmt.Errorf("template %s not found", tmplName)
	}
	
	srv.mux.HandleFunc(pattern,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			
			data := dataFunc(r)
			if err := srv.templates.ExecuteTemplate(w, tmplName, data); err != nil {
				logger.Error("Failed to execute template", "template", tmplName, "error", err)
				http.Error(w, "Error rendering template", http.StatusInternalServerError)
				return
			}
		})
	return nil
}

// EnsureTrailingSlash ensures that a directory path ends with a trailing slash.
// This utility function is used to normalize directory paths for consistent handling.
func EnsureTrailingSlash(dir string) string {
	if dir != "" && !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return dir
}

// HandleStatic registers a handler for serving static files from the configured static directory.
// The pattern should typically end with a wildcard (e.g., "/static/").
// Uses os.Root for secure file access when available (Go 1.24+).
func (srv *Server) HandleStatic(pattern string) {
	// Lazy initialization of static root on first use
	if srv.staticRoot == nil && srv.Options.StaticDir != "" {
		staticRoot, err := os.OpenRoot(srv.Options.StaticDir)
		if err != nil {
			logger.Warn("Failed to open static root directory, falling back to http.Dir", "error", err, "dir", srv.Options.StaticDir)
		} else {
			srv.staticRoot = staticRoot
			logger.Info("Static root initialized", "dir", srv.Options.StaticDir)
		}
	}

	if srv.staticRoot != nil {
		// Use secure os.Root with custom handler
		srv.mux.Handle(pattern, http.StripPrefix(pattern, srv.rootFileServer()))
		logger.Info("Static file serving using secure os.Root", "pattern", pattern)
	} else {
		// Fallback to traditional file server
		staticDir := EnsureTrailingSlash(srv.Options.StaticDir)
		srv.mux.Handle(pattern, http.StripPrefix(pattern, http.FileServer(http.Dir(staticDir))))
		logger.Info("Static file serving using http.Dir", "pattern", pattern, "dir", staticDir)
	}
}

// rootFileServer creates an http.Handler that serves files from os.Root
func (srv *Server) rootFileServer() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Clean the path
		path := filepath.Clean(r.URL.Path)
		if path == "/" {
			path = "index.html"
		}

		// Open file using os.Root
		file, err := srv.staticRoot.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				logger.Error("Failed to open file", "path", path, "error", err)
			}
			return
		}
		defer file.Close()

		// Get file info
		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Serve the file
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
	})
}

// HandleTemplate registers a handler that renders a specific template with static data.
// Unlike HandleFuncDynamic, the data is provided once at registration time.
// Returns an error if template parsing fails.
func (srv *Server) HandleTemplate(pattern, t string, data interface{}) error {
	if err := srv.parseTemplates(); err != nil {
		return fmt.Errorf("Failed to parse templates. %w", err)
	}
	
	// Check if the template exists
	if srv.templates != nil && srv.templates.Lookup(t) == nil {
		return fmt.Errorf("template %s not found", t)
	}
	
	srv.mux.HandleFunc(pattern, srv.templateHandler(t, data))
	return nil
}

func (srv *Server) parseTemplates() error {
	// Lock the mutex to prevent concurrent access to the templates
	srv.templatesMu.Lock()
	defer srv.templatesMu.Unlock()

	if srv.templates != nil {
		// Templates already parsed
		return nil
	}

	if srv.templateRoot != nil {
		// Use secure os.Root for template parsing (Go 1.24+)
		tmpl := template.New("root")

		// List directory contents using a helper function
		templateFiles, err := srv.listTemplateFiles()
		if err != nil {
			return fmt.Errorf("failed to list template files: %w", err)
		}

		for _, filename := range templateFiles {
			if strings.HasSuffix(filename, ".html") {
				// Open and read the template file
				file, err := srv.templateRoot.Open(filename)
				if err != nil {
					logger.Error("Failed to open template file", "file", filename, "error", err)
					continue
				}

				content, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					logger.Error("Failed to read template file", "file", filename, "error", err)
					continue
				}

				_, err = tmpl.New(filename).Parse(string(content))
				if err != nil {
					logger.Error("Failed to parse template", "file", filename, "error", err)
					return fmt.Errorf("failed to parse template %s: %w", filename, err)
				}
			}
		}

		srv.templates = tmpl
		logger.Info("Templates parsed using secure os.Root", "count", len(tmpl.Templates())-1) // -1 for root template
		return nil
	}

	// Fallback to traditional template parsing
	templateDir := srv.Options.TemplateDir
	// Check if the template directory exists
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		wd, _ := os.Getwd()
		ad, _ := filepath.Abs(templateDir)
		return fmt.Errorf("template directory not found. working-dir %s abs-path: %s, error %w", wd, ad, err)
	}

	// Parse the templates
	tmpl, err := template.ParseGlob(filepath.Join(templateDir, "*.html"))
	if err != nil {
		logger.Error("Failed to parse templates", "error", err)
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	srv.templates = tmpl
	logger.Info("Templates parsed.", "pattern", filepath.Join(templateDir, "*.html"))
	return nil
}

// DataFunc is a function type that generates data for template rendering.
// It receives the current HTTP request and returns data to be passed to the template.
type DataFunc func(r *http.Request) interface{}

// listTemplateFiles lists all files in the template root directory
func (srv *Server) listTemplateFiles() ([]string, error) {
	// Since os.Root doesn't have ReadDir, we need to use the regular os package
	// to list files, then validate them through os.Root
	var files []string

	// Read the actual directory
	entries, err := os.ReadDir(srv.Options.TemplateDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			// Verify we can open it through os.Root (validates it's within root)
			file, err := srv.templateRoot.Open(entry.Name())
			if err == nil {
				file.Close()
				files = append(files, entry.Name())
			}
		}
	}

	return files, nil
}

func checkfile(file, wd string) error {
	if _, err := os.Stat(file); err != nil {
		return fmt.Errorf("File %s not found in working directory %s. %w ", file, wd, err)
	}
	return nil
}

// WithTLS enables TLS on the server with the specified certificate and key files.
// Returns a ServerOptionFunc that configures TLS settings and validates file existence.
func WithTLS(certFile, keyFile string) ServerOptionFunc {

	return func(srv *Server) error {
		wd, _ := os.Getwd()
		// do not override existing values if not set
		if certFile != "" {
			srv.Options.CertFile = certFile
		}
		if keyFile != "" {
			srv.Options.KeyFile = keyFile
		}
		// check if the files exist
		errCert := checkfile(certFile, wd)
		errKey := checkfile(keyFile, wd)
		if errCert != nil || errKey != nil {
			return fmt.Errorf("Error checking files: %w %w", errCert, errKey)
		}
		srv.Options.EnableTLS = true
		return nil
	}
}

// WithLoglevel sets the global log level for the server.
// Accepts slog.Level values (LevelDebug, LevelInfo, LevelWarn, LevelError).
func WithLoglevel(level slog.Level) ServerOptionFunc {
	return func(srv *Server) error {
		slog.SetLogLoggerLevel(level)
		return nil
	}
}

// WithHealthServer enables the health server on a separate port.
// The health server provides /healthz/, /readyz/, and /livez/ endpoints for monitoring.
func WithHealthServer() ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.RunHealthServer = true
		return nil
	}
}

// WithAddr sets the address and port for the server to listen on.
// The address must be in the format "host:port" (e.g., ":8080", "localhost:3000").
func WithAddr(addr string) ServerOptionFunc {
	return func(srv *Server) error {
		// validate the address
		if _, _, err := net.SplitHostPort(addr); err != nil {
			logger.Error("setting address option", "error", err)
			// if the address failed to set, we must exit (no fallback to default etc.)
			return err
		}
		srv.Options.Addr = addr
		return nil
	}
}

// WithLogger replaces the default logger with a custom slog.Logger instance.
// This allows for custom log formatting, output destinations, and log levels.
func WithLogger(l *slog.Logger) ServerOptionFunc {
	return func(srv *Server) error {
		logger = l
		return nil
	}
}

// WithTimeouts configures the HTTP server timeouts.
// readTimeout: maximum duration for reading the entire request
// writeTimeout: maximum duration before timing out writes of the response
// idleTimeout: maximum time to wait for the next request when keep-alives are enabled
func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) ServerOptionFunc {
	return func(srv *Server) error {
		srv.setTimeouts(readTimeout, writeTimeout, idleTimeout)
		return nil
	}
}

// WithRateLimit configures rate limiting for the server.
// limit: maximum number of requests per second per client IP
// burst: maximum number of requests that can be made in a short burst
func WithRateLimit(limit rateLimit, burst int) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.RateLimit = limit
		srv.Options.Burst = burst
		return nil
	}
}

// WithTemplateDir sets the directory path where HTML templates are located.
// Templates in this directory can be used with HandleTemplate and HandleFuncDynamic methods.
// Returns an error if the specified directory does not exist or is not accessible.
func WithTemplateDir(dir string) ServerOptionFunc {
	return func(srv *Server) error {
		// Check if the directory exists and is accessible
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("template directory not found: %s", dir)
			}
			return fmt.Errorf("template directory access error: %s: %w", dir, err)
		}
		
		srv.Options.TemplateDir = dir
		return nil
	}
}

// WithAuthTokenValidator sets the token validator for the server.
func WithAuthTokenValidator(validator func(token string) (bool, error)) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.AuthTokenValidatorFunc = validator
		return nil
	}
}

// WithFIPSMode enables FIPS 140-3 compliant mode for government and enterprise deployments.
// This restricts TLS cipher suites and curves to FIPS-approved algorithms only.
func WithFIPSMode() ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.FIPSMode = true
		logger.Info("FIPS 140-3 mode enabled")
		return nil
	}
}

// WithHardenedMode enables hardened security mode for enhanced security headers.
// In hardened mode, the server header is suppressed and additional security measures are applied.
func WithHardenedMode() ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.HardenedMode = true
		logger.Info("Hardened security mode enabled")
		return nil
	}
}

// WithEncryptedClientHello enables Encrypted Client Hello (ECH) for enhanced privacy.
// ECH encrypts the SNI in TLS handshakes to prevent eavesdropping on the server name.
func WithEncryptedClientHello(echKeys ...[]byte) ServerOptionFunc {
	return func(srv *Server) error {
		if len(echKeys) == 0 {
			return fmt.Errorf("ECH requires at least one key")
		}
		srv.Options.EnableECH = true
		srv.Options.ECHKeys = echKeys
		logger.Info("Encrypted Client Hello enabled", "keyCount", len(echKeys))
		return nil
	}
}

// WithMCPSupport enables MCP (Model Context Protocol) support on the server.
// This allows AI assistants to connect and use tools/resources provided by the server.
// By default, MCP uses HTTP transport on the "/mcp" endpoint.
// Pass MCPOverHTTP() or MCPOverStdio() to configure the transport.
func WithMCPSupport(configs ...MCPTransportConfig) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPEnabled = true
		
		// Apply transport configurations
		if len(configs) == 0 {
			// Default to HTTP transport on /mcp
			srv.Options.MCPTransport = HTTPTransport
			srv.Options.mcpTransportOpts.transport = HTTPTransport
			srv.Options.mcpTransportOpts.endpoint = srv.Options.MCPEndpoint
		} else {
			// Apply all transport configurations
			for _, cfg := range configs {
				cfg(&srv.Options.mcpTransportOpts)
			}
			srv.Options.MCPTransport = srv.Options.mcpTransportOpts.transport
			if srv.Options.mcpTransportOpts.endpoint != "" {
				srv.Options.MCPEndpoint = srv.Options.mcpTransportOpts.endpoint
			}
		}
		
		transportName := "HTTP"
		if srv.Options.MCPTransport == StdioTransport {
			transportName = "stdio"
		}
		logger.Info("MCP (Model Context Protocol) support enabled", "transport", transportName, "endpoint", srv.Options.MCPEndpoint)
		return nil
	}
}

// WithMCPEndpoint configures the MCP endpoint path.
// Default is "/mcp" if not specified.
func WithMCPEndpoint(endpoint string) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPEndpoint = endpoint
		logger.Info("MCP endpoint configured", "endpoint", endpoint)
		return nil
	}
}

// WithMCPServerInfo configures the MCP server identification.
// This information is returned to MCP clients during initialization.
func WithMCPServerInfo(name, version string) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPServerName = name
		srv.Options.MCPServerVersion = version
		logger.Info("MCP server info configured", "name", name, "version", version)
		return nil
	}
}

// WithMCPFileToolRoot configures a root directory for MCP file operations.
// If specified, file tools will be restricted to this directory using os.Root for security.
func WithMCPFileToolRoot(rootDir string) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPFileToolRoot = rootDir
		logger.Info("MCP file tool root configured", "root", rootDir)
		return nil
	}
}

// WithMCPToolsDisabled disables MCP tools.
// Resources will still be available if enabled.
func WithMCPToolsDisabled() ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPToolsEnabled = false
		logger.Info("MCP tools disabled")
		return nil
	}
}

// WithMCPResourcesDisabled disables MCP resources.
// Tools will still be available if enabled.
func WithMCPResourcesDisabled() ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPResourcesEnabled = false
		logger.Info("MCP resources disabled")
		return nil
	}
}

// cleanupRateLimiters runs periodically to clean up old rate limiters
// This prevents memory leaks from accumulating client IP rate limiters
func (srv *Server) cleanupRateLimiters() {
	for {
		select {
		case <-srv.cleanupTicker.C:
			now := time.Now()
			srv.limitersMu.Lock()
			// Clean up rate limiters that haven't been used in the last 10 minutes
			for ip, entry := range srv.clientLimiters {
				if now.Sub(entry.lastAccess) > 10*time.Minute {
					delete(srv.clientLimiters, ip)
					logger.Debug("Cleaned up rate limiter", "ip", ip)
				}
			}
			srv.limitersMu.Unlock()
		case <-srv.cleanupDone:
			return
		}
	}
}

// stopCleanup stops the rate limiter cleanup goroutine
func (srv *Server) stopCleanup() {
	if srv.cleanupTicker != nil {
		srv.cleanupTicker.Stop()
	}
	if srv.cleanupDone != nil {
		close(srv.cleanupDone)
	}
}

// MCPEnabled returns true if MCP support is enabled
func (srv *Server) MCPEnabled() bool {
	return srv.Options.MCPEnabled && srv.mcpHandler != nil
}

// RegisterMCPTool registers a custom MCP tool
// This must be called after server creation but before Run()
func (srv *Server) RegisterMCPTool(tool MCPTool) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	srv.mcpHandler.RegisterTool(tool)
	return nil
}

// RegisterMCPResource registers a custom MCP resource
// This must be called after server creation but before Run()
func (srv *Server) RegisterMCPResource(resource MCPResource) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	srv.mcpHandler.RegisterResource(resource)
	return nil
}
