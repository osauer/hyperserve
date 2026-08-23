// Copyright 2024 by Oliver Sauer
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

/*
Package server provides a lightweight, high-performance HTTP server framework
with minimal external dependencies (golang.org/x/time/rate for rate limiting only).

Key Features:
  - Zero configuration with sensible defaults
  - Built-in middleware for logging, recovery, and metrics
  - Graceful shutdown handling with application hooks
  - Health check endpoints for Kubernetes
  - Model Context Protocol (MCP) support for AI assistants
  - WebSocket support for real-time communication (standard library only)
  - TLS/HTTPS support with automatic certificate management
  - Rate limiting and authentication
  - Template rendering support
  - Server-Sent Events (SSE) support

Basic Usage:

	srv, err := server.NewServer()
	if err != nil {
		log.Fatal(err)
	}

	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	srv.Run() // Blocks until shutdown signal

With Options:

	srv, err := server.NewServer(
		server.WithAddr(":8080"),
		server.WithHealthServer(),
		server.WithTLS("cert.pem", "key.pem"),
		server.WithMCPSupport("MyApp", "1.0.0"),
	)

Graceful Shutdown with Hooks:

	srv, err := server.NewServer(
		server.WithAddr(":8080"),
		server.WithOnShutdown(func(ctx context.Context) error {
			log.Println("Stopping background workers...")
			// Stop your application's goroutines, close connections, etc.
			return nil
		}),
	)

WebSocket Support (import websocket "github.com/osauer/hyperserve/pkg/websocket"):

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Configure based on your needs
		},
	}

	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// Handle WebSocket messages
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Echo message back
			conn.WriteMessage(messageType, p)
		}
	})
*/
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/osauer/hyperserve/pkg/mcp"
	"github.com/osauer/hyperserve/pkg/websocket"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	logger.Debug("Server initializing...")

	// If version is still "dev", try to get it from build info
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, dep := range info.Deps {
				if dep.Path == "github.com/osauer/hyperserve" {
					Version = dep.Version
					break
				}
			}
			// If we're the main module, use the Go version as a fallback
			if Version == "dev" && info.Main.Path == "github.com/osauer/hyperserve" {
				if info.Main.Version != "" && info.Main.Version != "(devel)" {
					Version = info.Main.Version
				}
			}
		}
	}
}

// Build information set at compile time using -ldflags
var (
	Version   = "dev"     // Version from git tags
	BuildHash = "unknown" // Git commit hash
	BuildTime = "unknown" // Build timestamp
)

// closeWithLog is a helper to handle Close errors in defer statements.
// Usage: defer closeWithLog(file, "file")
func closeWithLog(c io.Closer, name string) {
	if err := c.Close(); err != nil {
		logger.Warn("Failed to close resource", "resource", name, "error", err)
	}
}

// VersionInfo returns formatted version information
func VersionInfo() string {
	info := Version
	if BuildHash != "unknown" {
		info += "+" + BuildHash
	}
	if BuildTime != "unknown" {
		info += " (" + BuildTime + ")"
	}
	return info
}

// Environment management variable names
const (
	paramServerAddr           = "SERVER_ADDR"
	paramServerPort           = "HS_PORT"
	paramHealthAddr           = "HEALTH_ADDR"
	paramRateLimit            = "HS_RATE_LIMIT"
	paramBurstLimit           = "HS_BURST_LIMIT"
	paramServerHeader         = "HS_SERVER_HEADER"
	paramHardenedMode         = "HS_HARDENED_MODE"
	paramFileName             = "options.json"
	paramConfigPath           = "HS_CONFIG_PATH"
	paramMCPEnabled           = "HS_MCP_ENABLED"
	paramMCPEndpoint          = "HS_MCP_ENDPOINT"
	paramMCPServerName        = "HS_MCP_SERVER_NAME"
	paramMCPServerVersion     = "HS_MCP_SERVER_VERSION"
	paramMCPToolsEnabled      = "HS_MCP_TOOLS_ENABLED"
	paramMCPResourcesEnabled  = "HS_MCP_RESOURCES_ENABLED"
	paramMCPFileToolRoot      = "HS_MCP_FILE_TOOL_ROOT"
	paramMCPDev               = "HS_MCP_DEV"
	paramMCPObservability     = "HS_MCP_OBSERVABILITY"
	paramMCPTransport         = "HS_MCP_TRANSPORT"
	paramMCPProtocolVersion   = "HS_MCP_PROTOCOL_VERSION"
	paramCSPWebWorkerSupport  = "HS_CSP_WEB_WORKER_SUPPORT"
	paramCORSAllowedOrigins   = "HS_CORS_ALLOWED_ORIGINS"
	paramCORSAllowCredentials = "HS_CORS_ALLOW_CREDENTIALS"
	paramCORSAllowedMethods   = "HS_CORS_ALLOWED_METHODS"
	paramCORSAllowedHeaders   = "HS_CORS_ALLOWED_HEADERS"
	paramCORSExposeHeaders    = "HS_CORS_EXPOSE_HEADERS"
	paramCORSMaxAge           = "HS_CORS_MAX_AGE"
	paramLogLevel             = "HS_LOG_LEVEL"
	paramDebugMode            = "HS_DEBUG"
	paramStartupBanner        = "HS_STARTUP_BANNER"
	paramSuppressBanner       = "HS_SUPPRESS_BANNER"
	paramBannerColor          = "HS_BANNER_COLOR"
)

// RateLimit limits requests per second that can be requested from the httpServer. Requires to add [RateLimitMiddleware]
type RateLimit = rate.Limit

// rateLimiterEntry stores a rate limiter with last access time for cleanup.
// lastAccessUnixNano is accessed by both the request hot path (every request
// updates it) and the cleanup ticker (10-min stale eviction). Using an atomic
// lets the hot path skip the rate-limiter pool's write lock — the prior
// implementation took the pool's RWMutex.Lock() solely to bump a timestamp.
type rateLimiterEntry struct {
	limiter            *rate.Limiter
	lastAccessUnixNano atomic.Int64
}

// Server represents an HTTP server with built-in middleware support, health checks,
// template rendering, and various configuration options.
//
// The Server manages both the main HTTP server and an optional health check server.
// It handles graceful shutdown, request metrics, and can be extended with custom middleware.
//
// Example:
//
//	srv, _ := server.NewServer(
//		server.WithAddr(":8080"),
//		server.WithHealthServer(),
//	)
//
//	srv.HandleFunc("/api/users", handleUsers)
//	srv.Run()
type Server struct {
	mux                    *http.ServeMux
	healthMux              *http.ServeMux
	httpServer             *http.Server
	healthServer           *http.Server
	middleware             *middlewareRegistry
	templates              *template.Template
	templatesMu            sync.Mutex
	Options                *ServerOptions
	isReady                atomic.Bool
	isRunning              atomic.Bool
	totalRequests          atomic.Uint64
	totalResponseTime      atomic.Int64
	totalWebSocketUpgrades atomic.Uint64
	serverStart            time.Time
	routesMu               sync.RWMutex
	staticRoot             *os.Root
	templateRoot           *os.Root
	mcpHandler             *mcp.Handler
	rateLimiters           rateLimiterPool
	deferred               deferredLifecycle
}

// rateLimiterPool owns the per-client rate limiter map and its cleanup ticker.
type rateLimiterPool struct {
	clients       map[string]*rateLimiterEntry
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
	cleanupDone   chan bool
	stopOnce      sync.Once // Idempotent stopCleanup; covers convergent shutdown paths.
}

// deferredLifecycle tracks the state machine for WithDeferredInit / WithOnReady:
// before the listener flips to ready, application handlers return 503 and only
// bootstrap-allowed paths (e.g. /healthz) serve traffic.
type deferredLifecycle struct {
	init            func(context.Context, *Server) error
	initCancel      context.CancelFunc
	errMu           sync.RWMutex
	initErr         error
	ctx             context.Context
	cancel          context.CancelFunc
	bootstrapAllow  map[string]struct{}
	routes          map[string]struct{}
	onReadyMu       sync.Mutex
	onReadyExecuted atomic.Bool
}

// NewServer creates a new instance of the Server with the given options.
// By default, the server includes request logging, panic recovery, and metrics collection middleware.
// The server will listen on ":8080" unless configured otherwise.
//
// Options can be provided to customize the server behavior:
//
//	srv, err := server.NewServer(
//		server.WithAddr(":3000"),
//		server.WithHealthServer(),          // Enable health checks on :8081
//		server.WithTLS("cert.pem", "key.pem"), // Enable HTTPS
//		server.WithRateLimit(100, 200),     // 100 req/s, burst of 200
//	)
//
// Returns an error if any of the options fail to apply.
func NewServer(opts ...ServerOptionFunc) (*Server, error) {
	srv := newServerSkeleton()

	srv.middleware = newMiddlewareRegistry(DefaultMiddleware(srv))
	logger.Debug("Default middleware registered", "middlewares", []string{"MetricsMiddleware", "RequestLoggerMiddleware", "RecoveryMiddleware"})

	for _, opt := range opts {
		if err := opt(srv); err != nil {
			return nil, err
		}
	}

	if err := normalizeServerOptions(srv.Options); err != nil {
		return nil, err
	}
	applyConfiguredLogLevel(srv.Options)

	if err := autoConfigureMCP(srv); err != nil {
		return nil, err
	}
	if err := validateMCPProtocolVersion(srv.Options); err != nil {
		return nil, err
	}

	openTemplateRoot(srv)

	if srv.Options.MCPEnabled {
		initializeMCPHandler(srv)
	}

	// Start cleanup ticker for rate limiters (run every 5 minutes)
	srv.rateLimiters.cleanupTicker = time.NewTicker(5 * time.Minute)
	go srv.cleanupRateLimiters()

	srv.isReady.Store(srv.deferred.init == nil)
	return srv, nil
}

// newServerSkeleton allocates the fields the rest of NewServer expects to
// find non-nil — mux, options, rate-limiter pool, deferred-init bookkeeping.
func newServerSkeleton() *Server {
	options := DefaultServerOptions()
	return &Server{
		mux:     http.NewServeMux(),
		Options: &options,
		rateLimiters: rateLimiterPool{
			clients:     make(map[string]*rateLimiterEntry),
			cleanupDone: make(chan bool),
		},
		deferred: deferredLifecycle{
			bootstrapAllow: map[string]struct{}{
				"/healthz": {},
				"/readyz":  {},
				"/livez":   {},
			},
			routes: make(map[string]struct{}),
		},
	}
}

// applyConfiguredLogLevel maps the fully bound LogLevel string (or DebugMode
// flag) onto slog's default logger. Configuration sources and caller options
// have all run before this process-global side effect occurs.
func applyConfiguredLogLevel(opts *ServerOptions) {
	if opts.LogLevel != "" {
		switch opts.LogLevel {
		case "DEBUG":
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "INFO":
			slog.SetLogLoggerLevel(slog.LevelInfo)
		case "WARN":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "ERROR":
			slog.SetLogLoggerLevel(slog.LevelError)
		default:
			logger.Warn("Unknown log level, using INFO", "level", opts.LogLevel)
			slog.SetLogLoggerLevel(slog.LevelInfo)
		}
	}
	if opts.DebugMode {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		logger.Debug("Debug mode enabled from configuration")
	}
}

// autoConfigureMCP handles the "resolved options asked for MCP but no
// WithMCPSupport was called" path. WithMCPSupport with explicit modes wins.
func autoConfigureMCP(srv *Server) error {
	if !srv.Options.MCPEnabled || srv.Options.MCPServerName == "" || srv.mcpHandler != nil {
		return nil
	}
	if srv.Options.mcpTransportOpts.DeveloperMode || srv.Options.mcpTransportOpts.ObservabilityMode {
		logger.Debug("MCP already configured programmatically, skipping auto-configuration")
		return nil
	}
	if !srv.Options.MCPDev && !srv.Options.MCPObservability {
		return nil
	}

	var mcpConfigs []mcp.TransportConfig
	if srv.Options.MCPTransport == mcp.StdioTransport {
		mcpConfigs = append(mcpConfigs, mcp.OverStdio())
	}
	if srv.Options.MCPDev {
		mcpConfigs = append(mcpConfigs, MCPDev())
	}
	if srv.Options.MCPObservability {
		mcpConfigs = append(mcpConfigs, MCPObservability())
	}

	if err := WithMCPSupport(srv.Options.MCPServerName, srv.Options.MCPServerVersion, mcpConfigs...)(srv); err != nil {
		return fmt.Errorf("failed to auto-configure MCP: %w", err)
	}
	logger.Info("MCP auto-configured from resolved options",
		"name", srv.Options.MCPServerName,
		"transport", srv.Options.MCPTransport,
		"dev", srv.Options.MCPDev,
		"observability", srv.Options.MCPObservability)
	return nil
}

// initializeMCPHandler builds the MCP handler, fires the builtin-preset
// hooks (registered by `pkg/mcp/builtin` blank-imports), and registers the
// unified MCP endpoint + discovery routes on the mux.
func initializeMCPHandler(srv *Server) {
	serverInfo := mcp.ServerInfo{
		Name:    srv.Options.MCPServerName,
		Version: srv.Options.MCPServerVersion,
	}
	srv.mcpHandler = mcp.NewHandler(serverInfo)
	// The MCP handler owns its logging chain. Seed it from the logger injected
	// into the server package, then let presets wrap only this handler without
	// replacing process-wide defaults.
	srv.mcpHandler.SetLogger(logger)
	srv.mcpHandler.SetProtocolVersion(srv.Options.MCPProtocolVersion)
	srv.mcpHandler.SetToolCallTimeout(srv.Options.MCPToolCallTimeout)
	srv.mcpHandler.SetOriginValidator(srv.Options.MCPOriginValidator)
	//lint:ignore SA1019 Server wiring must apply the explicit legacy compatibility option.
	srv.mcpHandler.SetLegacyRoutedSSEEnabled(srv.Options.MCPLegacyRoutedSSE)

	if srv.Options.mcpTransportOpts.DeveloperMode {
		logger.Warn("⚠️  MCP DEVELOPER MODE ENABLED ⚠️",
			"warning", "This mode allows server restart and configuration changes",
			"security", "Only use in development environments")
	}
	if srv.Options.MCPToolsEnabled {
		if builtinToolsHook != nil {
			builtinToolsHook(srv)
		} else {
			logger.Warn("WithMCPBuiltinTools(true) was set but no builtin tools are registered",
				"reason", "missing blank import",
				"fix", `add: _ "github.com/osauer/hyperserve/pkg/mcp/builtin"`)
		}
	}
	if srv.Options.MCPResourcesEnabled {
		switch {
		case srv.Options.mcpTransportOpts.ObservabilityMode && builtinObservabilityHook != nil:
			builtinObservabilityHook(srv)
		case srv.Options.mcpTransportOpts.DeveloperMode && builtinDeveloperHook != nil:
			builtinDeveloperHook(srv)
		case builtinStandardResourcesHook != nil:
			builtinStandardResourcesHook(srv)
		default:
			logger.Warn("WithMCPBuiltinResources(true) was set but no builtin resources are registered",
				"reason", "missing blank import",
				"fix", `add: _ "github.com/osauer/hyperserve/pkg/mcp/builtin"`)
		}
	}

	srv.registerRoute(srv.Options.MCPEndpoint)
	srv.mux.Handle(srv.Options.MCPEndpoint, srv.mcpHandler)
	logger.Debug("MCP handler initialized", "endpoint", srv.Options.MCPEndpoint)

	srv.setupDiscoveryEndpoints()
}

func validateMCPProtocolVersion(options *ServerOptions) error {
	version := strings.TrimSpace(options.MCPProtocolVersion)
	if version == "" {
		options.MCPProtocolVersion = mcp.DefaultProtocolVersion
		return nil
	}
	if version == mcp.StreamableHTTPProtocolVersion {
		return fmt.Errorf("MCP protocol version %s is selected per Streamable HTTP request and cannot be configured as the initialize-era version", version)
	}
	options.MCPProtocolVersion = version
	return nil
}

// Run starts the server and blocks until a shutdown signal is received.
// It automatically:
//   - Starts the main HTTP/HTTPS server
//   - Starts the health check server (if enabled)
//   - Sets up graceful shutdown on SIGINT/SIGTERM
//   - Handles cleanup of resources
//   - Waits for active requests to complete before shutting down
//
// The method will block until the server is shut down, either by signal or error.
// Returns an error if the server fails to start or encounters a fatal error.
//
// Example:
//
//	if err := srv.Run(); err != nil {
//	    log.Fatal("Server failed:", err)
//	}
func (srv *Server) Run() error {
	// MCP stdio owns its lifecycle through stdin EOF. Registering for signals
	// here would consume them without giving the blocking read loop a way to
	// stop, so preserve the transport's existing EOF-only behavior.
	if srv.Options.MCPEnabled && srv.Options.MCPTransport == mcp.StdioTransport {
		return srv.run(context.Background())
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	return srv.run(ctx)
}

// RunContext starts the HTTP/HTTPS server and blocks until ctx requests a
// graceful shutdown, the server exits, or deferred initialization fails. It
// does not subscribe to process signals; the caller owns the lifecycle.
// Cancellation is a normal shutdown trigger and returns nil when shutdown
// succeeds. RunContext returns an error for MCP stdio transport because a
// context cannot portably interrupt its blocking stdin read; use Run and EOF.
// A Server must not be run concurrently or reused after RunContext returns.
func (srv *Server) RunContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("hyperserve: RunContext called with nil context")
	}
	if srv.Options.MCPEnabled && srv.Options.MCPTransport == mcp.StdioTransport {
		return errors.New("hyperserve: RunContext does not support MCP stdio transport; use Run and close stdin")
	}
	if ctx.Err() != nil {
		logger.Info("Server context already done; skipping startup.", "reason", context.Cause(ctx))
		return srv.shutdownAfter(nil)
	}
	return srv.run(ctx)
}

// run contains the transport startup shared by Run and RunContext. triggerCtx
// begins shutdown but deliberately does not become the HTTP BaseContext: the
// server's internal lifecycle context preserves the existing request and
// deferred-initialization semantics.
func (srv *Server) run(triggerCtx context.Context) error {
	// Print ASCII art on startup (skip in stdio mode or if suppressed)
	if srv.Options.MCPTransport != mcp.StdioTransport && !srv.Options.SuppressBanner {
		srv.printStartupBanner()
	}

	// log httpServer start time for collection up-time metric
	srv.serverStart = time.Now()

	// Check if we're running in stdio mode for MCP
	if srv.Options.MCPEnabled && srv.Options.MCPTransport == mcp.StdioTransport {
		if srv.deferred.init != nil {
			logger.Warn("Deferred initialization is not supported in MCP stdio transport; ignoring configuration")
		}
		// Run MCP in stdio mode
		if srv.mcpHandler == nil {
			return fmt.Errorf("MCP handler not initialized for stdio transport")
		}
		srv.isRunning.Store(true)
		return srv.mcpHandler.RunStdioLoop()
	}

	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	srv.deferred.ctx = lifecycleCtx
	srv.deferred.cancel = lifecycleCancel

	baseHandler := srv.middleware.applyToMux(srv.mux)
	if srv.deferred.init != nil {
		baseHandler = srv.bootstrapReadinessHandler(baseHandler)
	}

	// initialize the underlying http httpServer for serving requests
	srv.httpServer = &http.Server{
		Handler:           baseHandler,
		ReadTimeout:       srv.Options.ReadTimeout,
		WriteTimeout:      srv.Options.WriteTimeout,
		IdleTimeout:       srv.Options.IdleTimeout,
		ReadHeaderTimeout: srv.Options.ReadHeaderTimeout, // Prevent Slowloris attacks
		BaseContext: func(_ net.Listener) context.Context {
			return lifecycleCtx
		},
	}

	// If ReadHeaderTimeout is not set, default to ReadTimeout
	if srv.httpServer.ReadHeaderTimeout == 0 && srv.httpServer.ReadTimeout > 0 {
		srv.httpServer.ReadHeaderTimeout = srv.httpServer.ReadTimeout
	}
	srv.httpServer.RegisterOnShutdown(srv.logServerMetrics)

	if srv.Options.RunHealthServer {
		err := srv.initHealthServer()
		if err != nil {
			return err
		}
	}

	// Channel for server errors
	serverErr := make(chan error, 1)
	var deferredErr chan error
	if srv.deferred.init != nil {
		deferredErr = make(chan error, 1)
	}

	var listener net.Listener
	var listenErr error

	if srv.Options.EnableTLS {
		if srv.Options.CertFile == "" || srv.Options.KeyFile == "" {
			listenErr = fmt.Errorf("TLS enabled but no key or cert file provided")
			logger.Error(listenErr.Error(), "key", srv.Options.KeyFile, "cert", srv.Options.CertFile)
			return listenErr
		}
		// Configure TLS settings
		srv.httpServer.TLSConfig = srv.tlsConfig()
		srv.httpServer.Addr = srv.Options.TLSAddr
		listener, listenErr = net.Listen("tcp", srv.Options.TLSAddr)
		if listenErr != nil {
			return fmt.Errorf("failed to listen on %s: %w", srv.Options.TLSAddr, listenErr)
		}
	} else {
		srv.httpServer.Addr = srv.Options.Addr
		listener, listenErr = net.Listen("tcp", srv.Options.Addr)
		if listenErr != nil {
			return fmt.Errorf("failed to listen on %s: %w", srv.Options.Addr, listenErr)
		}
	}

	// Run the server in a goroutine
	go func(enableTLS bool, ln net.Listener) {
		var serveErr error
		if enableTLS {
			tlsListener := tls.NewListener(ln, srv.httpServer.TLSConfig)
			serveErr = srv.httpServer.Serve(tlsListener)
		} else {
			serveErr = srv.httpServer.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("HTTP server encountered an error", "error", serveErr)
		}
		serverErr <- serveErr
	}(srv.Options.EnableTLS, listener)

	// Mark as running only AFTER all servers (http AND health) are initialized
	srv.isRunning.Store(true)

	if srv.deferred.init != nil {
		srv.startDeferredInit(deferredErr)
	}

	// Graceful shutdown handling
	return srv.handleShutdown(triggerCtx, serverErr, deferredErr)
}

func (srv *Server) logServerMetrics() {
	totalReq := srv.totalRequests.Load()
	totalUs := srv.totalResponseTime.Load() // microseconds, accumulated across all handlers
	// avg µs per handled request. The prior shape divided the other way and
	// reported a uint64 floor of "requests per µs", which collapses to 0 for
	// any realistic workload.
	avgUs := int64(0)
	if totalReq > 0 {
		avgUs = totalUs / int64(totalReq)
	}
	upTime := time.Since(srv.serverStart)
	logger.Info("Server metrics:",
		"up-time", upTime,
		"µs-in-handlers", totalUs,
		"total-req", totalReq,
		"websocket-upgrades-total", srv.totalWebSocketUpgrades.Load(),
		"avg-µs-per-req", avgUs)
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
		logger.Info("TLS configured with FIPS-approved cipher suites and curves",
			"note", "this is not full FIPS 140-3 compliance; see WithFIPSMode docs")
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

	baseCtx := srv.deferred.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	srv.healthServer = &http.Server{
		Addr:              srv.Options.HealthAddr,
		Handler:           srv.healthMux,
		ReadTimeout:       srv.Options.ReadTimeout,
		WriteTimeout:      srv.Options.WriteTimeout,
		IdleTimeout:       srv.Options.IdleTimeout,
		ReadHeaderTimeout: srv.Options.ReadHeaderTimeout, // Prevent Slowloris attacks
		BaseContext: func(_ net.Listener) context.Context {
			return baseCtx
		},
	}
	// If ReadHeaderTimeout is not set, default to ReadTimeout
	if srv.healthServer.ReadHeaderTimeout == 0 && srv.healthServer.ReadTimeout > 0 {
		srv.healthServer.ReadHeaderTimeout = srv.healthServer.ReadTimeout
	}

	// Bind the listener synchronously so EADDRINUSE (and friends) surface
	// before this function returns. The previous shape called ListenAndServe
	// inside a goroutine and then guessed "started" after a 100 ms timer —
	// on a contended runner that timer could win a real `bind` error and we'd
	// claim success while the listener was dead. net.Listen + Serve(ln) is
	// what the main HTTP server already uses; the health server now matches.
	ln, err := net.Listen("tcp", srv.Options.HealthAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", srv.Options.HealthAddr, err)
	}

	go func() {
		logger.Debug("Starting health server", "addr", srv.Options.HealthAddr)
		if err := srv.healthServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Health server encountered an error", "error", err)
		}
	}()
	return nil
}

func (srv *Server) handleShutdown(triggerCtx context.Context, serverErr <-chan error, deferredErr <-chan error) error {
	// deferredErr may legitimately fire nil first (success); only a non-nil
	// error counts as a shutdown trigger.
	for {
		select {
		case <-triggerCtx.Done():
			logger.Info("Shutting down server.", "reason", context.Cause(triggerCtx))
			return srv.shutdownAfter(nil)
		case err := <-deferredErr:
			if err == nil {
				continue
			}
			logger.Error("Deferred initialization failed", "error", err)
			return srv.shutdownAfter(err)
		case err := <-serverErr:
			return srv.handleServerExit(err)
		}
	}
}

// shutdownAfter performs the standard pre-shutdown state flip plus
// `shutdown` with a 10s budget, and (when `originalErr` is non-nil) joins
// any shutdown error with the original cause.
func (srv *Server) shutdownAfter(originalErr error) error {
	srv.isReady.Store(false)
	srv.isRunning.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := srv.shutdown(ctx)
	if originalErr == nil {
		return shutdownErr
	}
	if shutdownErr != nil {
		return errors.Join(originalErr, fmt.Errorf("shutdown error: %w", shutdownErr))
	}
	return originalErr
}

// handleServerExit handles the case where the HTTP server's Serve returned
// — usually because we initiated shutdown, but possibly because the listen
// died. Cleans up rate-limiter resources unconditionally; rejoins the
// deferred-init error chain when present.
func (srv *Server) handleServerExit(err error) error {
	srv.isRunning.Store(false)
	srv.isReady.Store(false)
	if srv.deferred.cancel != nil {
		srv.deferred.cancel()
	}
	defer srv.stopCleanup()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if derr := srv.getDeferredInitError(); derr != nil && !errors.Is(derr, context.Canceled) {
		return derr
	}
	return err
}

func (srv *Server) startDeferredInit(errChan chan<- error) {
	ctx := srv.deferred.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	initCtx, cancel := context.WithCancel(ctx)
	srv.deferred.initCancel = cancel
	srv.isReady.Store(false)

	go func() {
		defer cancel()
		logger.Info("Deferred initialization started")
		if err := srv.deferred.init(initCtx, srv); err != nil {
			srv.completeDeferredInit(initCtx, err, errChan)
			return
		}

		if err := srv.completeDeferredInit(initCtx, nil, errChan); err != nil {
			return
		}
	}()
}

func (srv *Server) reportDeferredInitError(message string, err error, errChan chan<- error) {
	if err == nil {
		return
	}

	wrapped := fmt.Errorf("%s: %w", message, err)
	if errors.Is(err, context.Canceled) {
		logger.Warn(message, "error", err)
	} else {
		logger.Error(message, "error", err)
	}

	srv.setDeferredInitError(wrapped)
	srv.isReady.Store(false)

	shouldStop := srv.Options.StopOnDeferredInitFailure
	if errors.Is(err, context.Canceled) {
		// If context was cancelled because the server is shutting down, always unblock Run.
		shouldStop = true
	}

	if !shouldStop {
		logger.Warn("Deferred initialization failure will keep server in initializing state", "ready", false)
	}

	if shouldStop && errChan != nil {
		select {
		case errChan <- wrapped:
		default:
		}
	}
}

func (srv *Server) runOnReadyHooks(ctx context.Context) error {
	if len(srv.Options.OnReadyHooks) == 0 {
		return nil
	}

	hookCtx := ctx
	if hookCtx == nil {
		hookCtx = context.Background()
	}

	logger.Info("Executing OnReady hooks", "count", len(srv.Options.OnReadyHooks))
	for i, hook := range srv.Options.OnReadyHooks {
		if hook == nil {
			continue
		}
		if hookCtx.Err() != nil {
			return hookCtx.Err()
		}
		if err := hook(hookCtx, srv); err != nil {
			return fmt.Errorf("on ready hook %d failed: %w", i, err)
		}
	}
	logger.Info("OnReady hooks completed")
	return nil
}

func (srv *Server) runOnReadyOnce(ctx context.Context) error {
	if len(srv.Options.OnReadyHooks) == 0 {
		return nil
	}

	if srv.deferred.onReadyExecuted.Load() {
		return nil
	}

	srv.deferred.onReadyMu.Lock()
	defer srv.deferred.onReadyMu.Unlock()
	if srv.deferred.onReadyExecuted.Load() {
		return nil
	}

	if err := srv.runOnReadyHooks(ctx); err != nil {
		return err
	}

	srv.deferred.onReadyExecuted.Store(true)
	return nil
}

func (srv *Server) completeDeferredInit(ctx context.Context, initErr error, errChan chan<- error) error {
	if initErr != nil {
		srv.reportDeferredInitError("Deferred initialization failed", initErr, errChan)
		return initErr
	}

	if ctx == nil {
		ctx = srv.deferred.ctx
	}

	if err := srv.runOnReadyOnce(ctx); err != nil {
		srv.reportDeferredInitError("OnReady hook failed", err, errChan)
		return err
	}

	srv.setDeferredInitError(nil)
	srv.isReady.Store(true)
	logger.Info("Deferred initialization completed; server is ready")
	return nil
}

func (srv *Server) bootstrapReadinessHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if srv.isReady.Load() {
			next.ServeHTTP(w, r)
			return
		}

		if srv.isPathAllowedDuringBootstrap(r.URL.Path) && srv.routeRegisteredFor(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if srv.serveBootstrapHealth(w, r) {
			return
		}

		w.Header().Set("Retry-After", "5")
		writeErrorResponse(w, http.StatusServiceUnavailable, "service initializing")
	})
}

func (srv *Server) serveBootstrapHealth(w http.ResponseWriter, r *http.Request) bool {
	path := normalizeHealthPath(r.URL.Path)
	if path == "" {
		return false
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	switch path {
	case "/healthz":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	case "/readyz":
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("initializing"))
	case "/livez":
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("alive"))
	default:
		return false
	}

	return true
}

func (srv *Server) routeRegisteredFor(path string) bool {
	if srv.hasRoute(path) {
		return true
	}
	if before, ok := strings.CutSuffix(path, "/"); ok {
		trimmed := before
		if trimmed == "" {
			trimmed = "/"
		}
		return srv.hasRoute(trimmed)
	}
	return srv.hasRoute(path + "/")
}

func (srv *Server) isPathAllowedDuringBootstrap(path string) bool {
	normalized := normalizeHealthPath(path)
	if normalized == "" {
		return false
	}
	_, ok := srv.deferred.bootstrapAllow[normalized]
	return ok
}

func normalizeHealthPath(path string) string {
	if path == "" {
		return ""
	}
	if path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	if path == "" {
		path = "/"
	}
	switch path {
	case "/healthz", "/readyz", "/livez":
		return path
	}
	return ""
}

func (srv *Server) getDeferredInitError() error {
	srv.deferred.errMu.RLock()
	defer srv.deferred.errMu.RUnlock()
	return srv.deferred.initErr
}

func (srv *Server) setDeferredInitError(err error) {
	srv.deferred.errMu.Lock()
	srv.deferred.initErr = err
	srv.deferred.errMu.Unlock()
}

func (srv *Server) shutdown(ctx context.Context) error {
	var mcpShutdownErr error
	if srv.mcpHandler != nil {
		mcpCtx, cancelMCP := mcpShutdownContext(ctx)
		if err := srv.mcpHandler.Shutdown(mcpCtx); err != nil {
			mcpShutdownErr = fmt.Errorf("MCP handler shutdown error: %w", err)
			logger.Error("Error during MCP handler shutdown.", "error", err)
		}
		cancelMCP()
	}
	if srv.deferred.initCancel != nil {
		srv.deferred.initCancel()
	}
	if srv.deferred.cancel != nil {
		srv.deferred.cancel()
	}

	// Execute shutdown hooks first (before HTTP server shutdown)
	// Give hooks 5 seconds of the 10-second budget
	if len(srv.Options.OnShutdownHooks) > 0 {
		hookDeadline := 5 * time.Second
		// If overall deadline is shorter, use half of it for hooks
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < 10*time.Second {
				hookDeadline = remaining / 2
			}
		}

		hookCtx, hookCancel := context.WithTimeout(ctx, hookDeadline)
		defer hookCancel()

		logger.Info("Executing shutdown hooks", "count", len(srv.Options.OnShutdownHooks))
		for i, hook := range srv.Options.OnShutdownHooks {
			if hook == nil {
				continue
			}

			// Run hook with timeout
			done := make(chan error, 1)
			go func(h func(context.Context) error) {
				done <- h(hookCtx)
			}(hook)

			select {
			case err := <-done:
				if err != nil {
					logger.Error("Shutdown hook error", "hook", i, "error", err)
				} else {
					logger.Debug("Shutdown hook completed", "hook", i)
				}
			case <-hookCtx.Done():
				logger.Warn("Shutdown hook timeout", "hook", i)
				// Continue with remaining hooks even if one times out
			}
		}
		logger.Info("All shutdown hooks executed")
	}

	// Create an error channel to collect errors from goroutines
	errChan := make(chan error, 2)
	var wg sync.WaitGroup

	// Shutdown health server if it's running
	if srv.Options.RunHealthServer && srv.healthServer != nil {
		wg.Go(func() {
			logger.Info("Shutting down health server.")
			if err := srv.healthServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				logger.Error("Error during health server shutdown.", "error", err)
				errChan <- fmt.Errorf("health server shutdown error: %w", err)
			}
		})
	}

	// Shutdown http server
	if srv.httpServer != nil {
		wg.Go(func() {
			logger.Info("Shutting down http server.")
			if err := srv.httpServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				logger.Error("Error during main server shutdown.", "error", err)
				errChan <- fmt.Errorf("main server shutdown error: %w", err)
			}
		})
	}

	// Wait for both shutdowns to complete
	wg.Wait()
	close(errChan)

	// Collect errors. errors.Join preserves the full wrap chain of every
	// component error so callers can errors.Is/As against any of them.
	var shutdownErrs []error
	for err := range errChan {
		shutdownErrs = append(shutdownErrs, err)
	}
	shutdownErr := errors.Join(append([]error{mcpShutdownErr}, shutdownErrs...)...)

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

func mcpShutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	const maximum = 5 * time.Second
	timeout := maximum
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if half := remaining / 2; half < timeout {
			timeout = half
		}
	}
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

// WebSocketUpgrader returns a WebSocket upgrader that tracks the upgrade in
// server telemetry. Use this instead of a standalone Upgrader so WS upgrades
// land in totalWebSocketUpgrades alongside the totalRequests counter that
// MetricsMiddleware already maintains for every request.
func (srv *Server) WebSocketUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Default to same-origin policy
			return websocket.DefaultCheckOrigin(r)
		},
		BeforeUpgrade: func(w http.ResponseWriter, r *http.Request) error {
			// MetricsMiddleware already incremented totalRequests for this
			// request; only the WS-specific counter is ours to bump here.
			srv.totalWebSocketUpgrades.Add(1)
			return nil
		},
	}
}

// Stop gracefully stops the server with a default timeout of 10 seconds
func (srv *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.isReady.Store(false)
	srv.isRunning.Store(false)
	return srv.shutdown(ctx)
}

// CompleteDeferredInit allows applications to manually finalize deferred initialization
// after addressing failures. Passing a nil error reruns any pending OnReady hooks and
// marks the server ready. Passing a non-nil error records the failure and leaves the
// server in an initializing state.
func (srv *Server) CompleteDeferredInit(ctx context.Context, err error) error {
	return srv.completeDeferredInit(ctx, err, nil)
}

// Handle registers an http.Handler for the given pattern. Mirrors
// http.ServeMux.Handle but also tracks the pattern so middleware stacks
// applied via AddMiddlewareStack can find it. Use this when you have an
// existing http.Handler (e.g., http.FileServer); use HandleFunc for inline
// handler functions.
//
//	srv.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
func (srv *Server) Handle(pattern string, handler http.Handler) {
	srv.registerRoute(pattern)
	srv.mux.Handle(pattern, handler)
}

func (srv *Server) registerRoute(pattern string) {
	if pattern == "" {
		return
	}
	srv.routesMu.Lock()
	srv.deferred.routes[pattern] = struct{}{}
	srv.routesMu.Unlock()
}

func (srv *Server) hasRoute(pattern string) bool {
	srv.routesMu.RLock()
	_, ok := srv.deferred.routes[pattern]
	srv.routesMu.RUnlock()
	return ok
}

// HandleFunc registers the handler function for the given pattern.
// The pattern follows the standard net/http ServeMux patterns:
//   - "/path" matches exactly
//   - "/path/" matches the path and any subpaths
//   - Patterns are matched in order of specificity
//
// Registered handlers automatically benefit from any global middleware
// (logging, recovery, metrics) plus any route-specific middleware.
//
// Example:
//
//	srv.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
//	    users := getUsersFromDB()
//	    json.NewEncoder(w).Encode(users)
//	})
//
//	srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
//	    w.WriteHeader(http.StatusOK)
//	    fmt.Fprintln(w, "OK")
//	})

func (srv *Server) Handler() http.Handler {
	return srv.middleware.applyToMux(srv.mux)
}

func (srv *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	srv.registerRoute(pattern)
	srv.mux.HandleFunc(pattern, handler)
}

// GET, POST, PUT, PATCH, DELETE, HEAD, and OPTIONS are method-aware shortcuts
// for HandleFunc. They prepend the method to the pattern, relying on the
// net/http 1.22+ "METHOD /path" syntax — wrong-method requests are rejected
// by the mux with 405 Method Not Allowed, so handlers no longer need to
// switch on r.Method themselves. Pattern wildcards (`/users/{id}`) work as
// usual via r.PathValue.
//
// HandleFunc remains the lower-level escape hatch for one handler covering
// all methods or for the legacy pattern syntax.

// GET registers handler for GET requests matching pattern.
func (srv *Server) GET(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodGet+" "+pattern, handler)
}

// POST registers handler for POST requests matching pattern.
func (srv *Server) POST(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodPost+" "+pattern, handler)
}

// PUT registers handler for PUT requests matching pattern.
func (srv *Server) PUT(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodPut+" "+pattern, handler)
}

// PATCH registers handler for PATCH requests matching pattern.
func (srv *Server) PATCH(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodPatch+" "+pattern, handler)
}

// DELETE registers handler for DELETE requests matching pattern.
func (srv *Server) DELETE(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodDelete+" "+pattern, handler)
}

// HEAD registers handler for HEAD requests matching pattern.
func (srv *Server) HEAD(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodHead+" "+pattern, handler)
}

// OPTIONS registers handler for OPTIONS requests matching pattern.
func (srv *Server) OPTIONS(pattern string, handler http.HandlerFunc) {
	srv.HandleFunc(http.MethodOptions+" "+pattern, handler)
}

func checkfile(file, wd string) error {
	if _, err := os.Stat(file); err != nil {
		return fmt.Errorf("file %s not found in working directory %s: %w", file, wd, err)
	}
	return nil
}

// cleanupRateLimiters runs periodically to clean up old rate limiters
// This prevents memory leaks from accumulating client IP rate limiters
func (srv *Server) cleanupRateLimiters() {
	ticker := srv.rateLimiters.cleanupTicker
	if ticker == nil {
		return
	}

	done := srv.rateLimiters.cleanupDone
	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute).UnixNano()
			srv.rateLimiters.mu.Lock()
			// Clean up rate limiters that haven't been used in the last 10 minutes
			for ip, entry := range srv.rateLimiters.clients {
				if entry.lastAccessUnixNano.Load() < cutoff {
					delete(srv.rateLimiters.clients, ip)
					logger.Debug("Cleaned up rate limiter", "ip", ip)
				}
			}
			srv.rateLimiters.mu.Unlock()
		case <-done:
			return
		}
	}
}

// stopCleanup stops the rate limiter cleanup goroutine. Idempotent via
// sync.Once: shutdown paths can converge (e.g. serverErr fires while a
// separate shutdownAfter has already run), and calling close() twice
// would otherwise panic. We intentionally do NOT nil the ticker/done
// fields — the cleanup goroutine reads them without locking, so writing
// to them after start would race the reader. The Once-guarded stop is
// enough; the goroutine exits via cleanupDone's close signal.
func (srv *Server) stopCleanup() {
	srv.rateLimiters.stopOnce.Do(func() {
		if srv.rateLimiters.cleanupTicker != nil {
			srv.rateLimiters.cleanupTicker.Stop()
		}
		if srv.rateLimiters.cleanupDone != nil {
			close(srv.rateLimiters.cleanupDone)
		}
	})
}

// =============================================================================
// Accessors for the builtin MCP tools/resources package
// =============================================================================
//
// pkg/mcp/builtin lives outside this package but needs read access to
// server runtime state to surface it via MCP. These accessors keep the
// internals encapsulated.

// IsRunning reports whether the server is currently running.
func (srv *Server) IsRunning() bool { return srv.isRunning.Load() }

// IsReady reports whether the server is ready to accept traffic.
func (srv *Server) IsReady() bool { return srv.isReady.Load() }

// ServerStart returns the timestamp when the server began serving.
func (srv *Server) ServerStart() time.Time { return srv.serverStart }

// TotalRequests returns the total number of requests served so far.
func (srv *Server) TotalRequests() uint64 { return srv.totalRequests.Load() }

// TotalResponseTime returns the cumulative response time in microseconds.
func (srv *Server) TotalResponseTime() int64 { return srv.totalResponseTime.Load() }

// SetMetrics is a test affordance: it overrides the request count and
// cumulative response time. Production code should never call this; metrics
// are populated by the request-handling middleware.
func (srv *Server) SetMetrics(totalRequests uint64, totalResponseTime int64) {
	srv.totalRequests.Store(totalRequests)
	srv.totalResponseTime.Store(totalResponseTime)
}

// AddMetrics is a test affordance: it bumps the request count by one and
// adds to the cumulative response time. See SetMetrics.
func (srv *Server) AddMetrics(deltaRequests uint64, deltaResponseTime int64) {
	srv.totalRequests.Add(deltaRequests)
	srv.totalResponseTime.Add(deltaResponseTime)
}

// ClientLimiterCount returns the number of active per-client rate limiters.
func (srv *Server) ClientLimiterCount() int {
	srv.rateLimiters.mu.RLock()
	defer srv.rateLimiters.mu.RUnlock()
	return len(srv.rateLimiters.clients)
}

// Mux returns the server's underlying http.ServeMux. Exposed so tests can
// mount the server's handler in an httptest server without going through
// (*Server).Run.
func (srv *Server) Mux() *http.ServeMux { return srv.mux }

// RegisteredRoutes returns a sorted snapshot of patterns registered through
// Handle, HandleFunc, and the method-aware route helpers.
func (srv *Server) RegisteredRoutes() []string {
	srv.routesMu.RLock()
	defer srv.routesMu.RUnlock()
	routes := make([]string, 0, len(srv.deferred.routes))
	for route := range srv.deferred.routes {
		routes = append(routes, route)
	}
	slices.Sort(routes)
	return routes
}

// MiddlewareRoutes returns a snapshot of the registered route-to-middleware
// mapping. The returned map is a shallow copy; mutating it does not affect the
// server.
func (srv *Server) MiddlewareRoutes() map[string]MiddlewareStack {
	if srv.middleware == nil {
		return nil
	}
	out := make(map[string]MiddlewareStack, len(srv.middleware.middleware))
	maps.Copy(out, srv.middleware.middleware)
	return out
}

// printStartupBanner prints the ASCII art and startup information
func (srv *Server) printStartupBanner() {
	// ASCII art for hyperserve (without color for terminal compatibility)
	banner := `
 _                                              
| |__  _   _ _ __   ___ _ __ ___  ___ _ ____   _____
| '_ \| | | | '_ \ / _ \ '__/ __|/ _ \ '__\ \ / / _ \
| | | | |_| | |_) |  __/ |  \__ \  __/ |   \ V /  __/
|_| |_|\__, | .__/ \___|_|  |___/\___|_|    \_/ \___|
       |___/|_|                                      
`
	if srv.Options.BannerColor {
		const bannerColor = "\033[35m"
		const reset = "\033[0m"
		fmt.Print(bannerColor, banner, reset)
	} else {
		fmt.Print(banner)
	}

	// Version and build information
	fmt.Printf("\nhyperserve %s", Version)
	if BuildHash != "unknown" || BuildTime != "unknown" {
		fmt.Printf(" (build: %s, %s)", BuildHash, BuildTime)
	}
	fmt.Println()

	// Build consolidated startup information
	addr := srv.Options.Addr
	if srv.Options.EnableTLS {
		addr = srv.Options.TLSAddr
	}

	protocol := "http"
	if srv.Options.EnableTLS {
		protocol = "https"
	}

	// Print consolidated startup info
	fmt.Printf("\nServer:    %s://%s\n", protocol, addr)

	if srv.Options.RunHealthServer {
		fmt.Printf("Health:    http://%s\n", srv.Options.HealthAddr)
	}

	if srv.Options.MCPEnabled {
		fmt.Printf("MCP:       %s (unified HTTP/SSE endpoint)\n", srv.Options.MCPEndpoint)
		if srv.Options.mcpTransportOpts.DeveloperMode {
			// Make MCP more discoverable for AI assistants
			fmt.Printf("\n🤖 MCP ENABLED: AI assistants connect via unified endpoint:\n")
			fmt.Printf("   SSE: GET %s://%s%s with Accept: text/event-stream\n", protocol, addr, srv.Options.MCPEndpoint)
			fmt.Printf("   HTTP: POST %s://%s%s with Content-Type: application/json\n", protocol, addr, srv.Options.MCPEndpoint)
		}
	}

	fmt.Println() // Empty line after banner
}
