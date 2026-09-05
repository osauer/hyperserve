// Copyright 2024 by Oliver Sauer
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package hyperserve

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/osauer/hyperserve/v2/mcp"
	"github.com/osauer/hyperserve/v2/websocket"
)

func init() {
	// If version is still "dev", try to get it from build info
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, dep := range info.Deps {
				if dep.Path == "github.com/osauer/hyperserve/v2" {
					Version = dep.Version
					break
				}
			}
			// If we're the main module, use the Go version as a fallback
			if Version == "dev" && info.Main.Path == "github.com/osauer/hyperserve/v2" {
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
	paramBannerColor          = "HS_BANNER_COLOR"
)

// Server represents an HTTP server with built-in middleware support, health checks,
// template rendering, and various configuration options.
//
// The Server manages both the main HTTP server and an optional health check server.
// It handles graceful shutdown, request metrics, and can be extended with custom middleware.
//
// Example:
//
//	app, _ := hyperserve.New(
//		hyperserve.WithAddr(":8080"),
//		hyperserve.WithHealthServer(),
//	)
//
//	app.HandleFunc("/api/users", handleUsers)
//	if err := app.Run(ctx); err != nil {
//		log.Fatal(err)
//	}
type Server struct {
	mux                    *http.ServeMux
	healthMux              *http.ServeMux
	httpServer             *http.Server
	healthServer           *http.Server
	listener               net.Listener
	healthListener         net.Listener
	middleware             *middlewareRegistry
	templates              *template.Template
	templatesMu            sync.Mutex
	options                Options
	logger                 *slog.Logger
	customLogger           bool
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
	deferred               deferredLifecycle
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

// New creates a Server with the given options.
// By default, the server includes request logging, panic recovery, and metrics collection middleware.
// The server will listen on ":8080" unless configured otherwise.
//
// Options can be provided to customize the server behavior:
//
//	app, err := hyperserve.New(
//		hyperserve.WithAddr(":3000"),
//		hyperserve.WithHealthServer(),             // Enable health checks on :9080
//		hyperserve.WithTLS("cert.pem", "key.pem"), // Enable HTTPS
//	)
//
// Returns an error if any of the options fail to apply.
func New(options ...Option) (*Server, error) {
	srv := newServerSkeleton()

	for _, opt := range options {
		if err := opt(srv); err != nil {
			return nil, err
		}
	}

	if err := normalizeOptions(&srv.options); err != nil {
		return nil, err
	}
	srv.applyConfiguredLogLevel()
	srv.middleware = newMiddlewareRegistry(defaultMiddleware(srv))
	srv.logger.Debug("Default middleware registered", "middlewares", []string{"MetricsMiddleware", "RequestLoggerMiddleware", "RecoveryMiddleware"})

	if err := autoConfigureMCP(srv); err != nil {
		return nil, err
	}
	if err := validateMCPProtocolVersion(&srv.options); err != nil {
		return nil, err
	}

	openTemplateRoot(srv)

	if srv.options.MCPEnabled {
		initializeMCPHandler(srv)
	}

	srv.isReady.Store(srv.deferred.init == nil)
	return srv, nil
}

// newServerSkeleton allocates the fields the rest of New expects to find
// non-nil: the mux, options, and deferred-init bookkeeping.
func newServerSkeleton() *Server {
	options := DefaultOptions()
	return &Server{
		mux:     http.NewServeMux(),
		options: options,
		logger:  slog.Default(),
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

// applyConfiguredLogLevel creates the server-owned default logger after all
// configuration has been bound. A caller-provided logger remains untouched.
func (srv *Server) applyConfiguredLogLevel() {
	if srv.customLogger {
		return
	}
	level := slog.LevelWarn
	if srv.options.DebugMode {
		level = slog.LevelDebug
	} else {
		switch srv.options.LogLevel {
		case "", "WARN":
		case "DEBUG":
			level = slog.LevelDebug
		case "INFO":
			level = slog.LevelInfo
		case "ERROR":
			level = slog.LevelError
		default:
			srv.logger.Warn("Unknown log level, using WARN", "level", srv.options.LogLevel)
		}
	}
	srv.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// autoConfigureMCP handles the "resolved options asked for MCP but no
// WithMCPSupport was called" path. WithMCPSupport with explicit modes wins.
func autoConfigureMCP(srv *Server) error {
	if !srv.options.MCPEnabled || srv.options.MCPServerName == "" || srv.mcpHandler != nil {
		return nil
	}
	if srv.options.mcpTransportOpts.DeveloperMode || srv.options.mcpTransportOpts.ObservabilityMode {
		srv.logger.Debug("MCP already configured programmatically, skipping auto-configuration")
		return nil
	}
	if !srv.options.MCPDev && !srv.options.MCPObservability {
		return nil
	}

	var mcpConfigs []mcp.TransportConfig
	if srv.options.MCPTransport == mcp.StdioTransport {
		mcpConfigs = append(mcpConfigs, mcp.OverStdio())
	}
	if srv.options.MCPDev {
		mcpConfigs = append(mcpConfigs, MCPDev())
	}
	if srv.options.MCPObservability {
		mcpConfigs = append(mcpConfigs, MCPObservability())
	}

	if err := WithMCPSupport(srv.options.MCPServerName, srv.options.MCPServerVersion, mcpConfigs...)(srv); err != nil {
		return fmt.Errorf("failed to auto-configure MCP: %w", err)
	}
	srv.logger.Info("MCP auto-configured from resolved options",
		"name", srv.options.MCPServerName,
		"transport", srv.options.MCPTransport,
		"dev", srv.options.MCPDev,
		"observability", srv.options.MCPObservability)
	return nil
}

// initializeMCPHandler builds the MCP handler, fires the builtin-preset
// hooks (registered by an explicit mcp/builtin blank import), and registers the
// unified MCP endpoint + discovery routes on the mux.
func initializeMCPHandler(srv *Server) {
	serverInfo := mcp.ServerInfo{
		Name:    srv.options.MCPServerName,
		Version: srv.options.MCPServerVersion,
	}
	srv.mcpHandler = mcp.NewHandler(serverInfo)
	// The MCP handler owns its logging chain. Seed it from the logger injected
	// into the server package, then let presets wrap only this handler without
	// replacing process-wide defaults.
	srv.mcpHandler.SetLogger(srv.logger)
	srv.mcpHandler.SetProtocolVersion(srv.options.MCPProtocolVersion)
	srv.mcpHandler.SetToolCallTimeout(srv.options.MCPToolCallTimeout)
	srv.mcpHandler.SetOriginValidator(srv.options.MCPOriginValidator)
	//lint:ignore SA1019 Server wiring must apply the explicit legacy compatibility option.
	srv.mcpHandler.SetLegacyRoutedSSEEnabled(srv.options.MCPLegacyRoutedSSE)

	if srv.options.mcpTransportOpts.DeveloperMode {
		srv.logger.Warn("⚠️  MCP DEVELOPER MODE ENABLED ⚠️",
			"warning", "This mode exposes runtime status, routes, middleware layout, and development logs",
			"security", "Only use in development environments")
	}
	if srv.options.MCPToolsEnabled {
		if builtinToolsHook != nil {
			builtinToolsHook(srv)
		} else {
			srv.logger.Warn("WithMCPBuiltinTools(true) was set but no builtin tools are registered",
				"reason", "missing blank import",
				"fix", `add: _ "github.com/osauer/hyperserve/v2/mcp/builtin"`)
		}
	}
	if srv.options.MCPResourcesEnabled {
		switch {
		case srv.options.mcpTransportOpts.ObservabilityMode && builtinObservabilityHook != nil:
			builtinObservabilityHook(srv)
		case srv.options.mcpTransportOpts.DeveloperMode && builtinDeveloperHook != nil:
			builtinDeveloperHook(srv)
		case builtinStandardResourcesHook != nil:
			builtinStandardResourcesHook(srv)
		default:
			srv.logger.Warn("WithMCPBuiltinResources(true) was set but no builtin resources are registered",
				"reason", "missing blank import",
				"fix", `add: _ "github.com/osauer/hyperserve/v2/mcp/builtin"`)
		}
	}

	srv.registerRoute(srv.options.MCPEndpoint)
	srv.mux.Handle(srv.options.MCPEndpoint, srv.mcpHandler)
	srv.logger.Debug("MCP handler initialized", "endpoint", srv.options.MCPEndpoint)

	srv.setupDiscoveryEndpoints()
}

func validateMCPProtocolVersion(options *Options) error {
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

// Run starts the HTTP/HTTPS server and blocks until ctx requests a
// graceful shutdown, the server exits, or deferred initialization fails. It
// does not subscribe to process signals; the application owns the lifecycle.
// The context is a shutdown trigger; its values are not installed as HTTP
// request values. Use middleware for request-scoped data.
// Cancellation is a normal shutdown trigger and returns nil when shutdown
// succeeds. Run returns an error for MCP stdio transport because a context
// cannot portably interrupt its blocking stdin read; use RunStdio instead.
// A Server must not be run concurrently or reused after Run returns.
func (srv *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("hyperserve: Run called with nil context")
	}
	if srv.options.MCPEnabled && srv.options.MCPTransport == mcp.StdioTransport {
		return errors.New("hyperserve: Run does not support MCP stdio transport; use RunStdio")
	}
	if ctx.Err() != nil {
		srv.logger.Info("Server context already done; skipping startup.", "reason", context.Cause(ctx))
		return srv.shutdownAfter(nil)
	}
	return srv.run(ctx)
}

// RunStdio runs an MCP stdio server until stdin reaches EOF. Stdio is kept
// separate from Run because an arbitrary io.Reader cannot be interrupted by a
// context without closing an object the application may own.
func (srv *Server) RunStdio() error {
	if !srv.options.MCPEnabled || srv.options.MCPTransport != mcp.StdioTransport {
		return errors.New("hyperserve: RunStdio requires MCP stdio transport")
	}
	return srv.shutdownAfter(srv.run(context.Background()))
}

// run contains the transport startup shared by Run and RunStdio. triggerCtx
// begins shutdown but deliberately does not become the HTTP BaseContext: the
// server's internal lifecycle context preserves the existing request and
// deferred-initialization semantics.
func (srv *Server) run(triggerCtx context.Context) error {
	// Print ASCII art on startup (skip in stdio mode or if suppressed)
	if srv.options.MCPTransport != mcp.StdioTransport && srv.options.StartupBanner {
		srv.printStartupBanner()
	}

	// log httpServer start time for collection up-time metric
	srv.serverStart = time.Now()

	// Check if we're running in stdio mode for MCP
	if srv.options.MCPEnabled && srv.options.MCPTransport == mcp.StdioTransport {
		if srv.deferred.init != nil {
			srv.logger.Warn("Deferred initialization is not supported in MCP stdio transport; ignoring configuration")
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

	// Run is the explicit configuration boundary, so it can serve the immutable
	// plan directly. Handler keeps a lazy wrapper because applications may call
	// Handler before finishing registration, as long as they do so before serve.
	baseHandler := srv.middleware.compile(srv.mux)
	if srv.deferred.init != nil {
		baseHandler = srv.bootstrapReadinessHandler(baseHandler)
	}

	// initialize the underlying http httpServer for serving requests
	srv.httpServer = &http.Server{
		Handler:           baseHandler,
		ReadTimeout:       srv.options.ReadTimeout,
		WriteTimeout:      srv.options.WriteTimeout,
		IdleTimeout:       srv.options.IdleTimeout,
		ReadHeaderTimeout: srv.options.ReadHeaderTimeout, // Prevent Slowloris attacks
		BaseContext: func(_ net.Listener) context.Context {
			return lifecycleCtx
		},
	}

	// If ReadHeaderTimeout is not set, default to ReadTimeout
	if srv.httpServer.ReadHeaderTimeout == 0 && srv.httpServer.ReadTimeout > 0 {
		srv.httpServer.ReadHeaderTimeout = srv.httpServer.ReadTimeout
	}
	srv.httpServer.RegisterOnShutdown(srv.logServerMetrics)
	if srv.options.EnableTLS {
		if srv.options.CertFile == "" || srv.options.KeyFile == "" {
			configErr := fmt.Errorf("TLS enabled but no key or cert file provided")
			srv.logger.Error(configErr.Error(), "key", srv.options.KeyFile, "cert", srv.options.CertFile)
			return srv.shutdownAfter(configErr)
		}
		certificate, err := tls.LoadX509KeyPair(srv.options.CertFile, srv.options.KeyFile)
		if err != nil {
			return srv.shutdownAfter(fmt.Errorf("load TLS certificate: %w", err))
		}
		srv.httpServer.TLSConfig = srv.tlsConfig()
		srv.httpServer.TLSConfig.Certificates = []tls.Certificate{certificate}
	}

	if srv.options.RunHealthServer {
		err := srv.initHealthServer()
		if err != nil {
			// Construction already owns cleanup resources, so even a failed first
			// bind must finish through the ordinary shutdown path.
			return srv.shutdownAfter(err)
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

	if srv.options.EnableTLS {
		srv.httpServer.Addr = srv.options.TLSAddr
		listener, listenErr = net.Listen("tcp", srv.options.TLSAddr)
		if listenErr != nil {
			// The health listener may already be serving at this point.
			return srv.shutdownAfter(fmt.Errorf("failed to listen on %s: %w", srv.options.TLSAddr, listenErr))
		}
	} else {
		srv.httpServer.Addr = srv.options.Addr
		listener, listenErr = net.Listen("tcp", srv.options.Addr)
		if listenErr != nil {
			// The health listener may already be serving at this point.
			return srv.shutdownAfter(fmt.Errorf("failed to listen on %s: %w", srv.options.Addr, listenErr))
		}
	}
	srv.listener = listener

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
			srv.logger.Error("HTTP server encountered an error", "error", serveErr)
		}
		serverErr <- serveErr
	}(srv.options.EnableTLS, listener)

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
	srv.logger.Info("Server metrics:",
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

	if srv.options.FIPSMode {
		// CipherSuites controls TLS 1.2 only. TLS 1.3 policy belongs to Go.
		config.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		}
		config.CurvePreferences = []tls.CurveID{
			tls.CurveP256,
			tls.CurveP384,
		}
		srv.logger.Info("TLS configured with AES-GCM for TLS 1.2 and P-256/P-384 curves",
			"note", "TLS 1.3 cipher policy requires application-enabled Go FIPS mode")
	} else {
		// Explicit TLS 1.2 suites; Go selects TLS 1.3 suites.
		config.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		}
		// CurvePreferences nil enables post-quantum X25519MLKEM768 by default in Go 1.24
		config.CurvePreferences = nil
	}

	return config
}

// Use registers middleware for every request. Middleware is applied in the
// order provided, with the first item outermost. Register middleware before
// calling Run or serving Handler; registration after serving starts panics.
func (srv *Server) Use(middleware ...Middleware) {
	srv.middleware.Add(globalMiddlewareRoute, middleware)
	srv.logger.Debug("Middleware registered", "scope", "global", "count", len(middleware))
}

// UsePrefix registers middleware for a URL path and its child paths at a
// slash boundary. For example, "/api" matches "/api/users" but not "/apiv2".
// A non-empty prefix must begin with "/"; malformed prefixes panic at
// registration time so a security middleware cannot be silently bypassed.
// Register middleware before calling Run or serving Handler; registration
// after serving starts panics.
func (srv *Server) UsePrefix(prefix string, middleware ...Middleware) {
	if err := validateMiddlewarePrefix(prefix); err != nil {
		panic("hyperserve: " + err.Error())
	}
	srv.middleware.Add(prefix, middleware)
	srv.logger.Debug("Middleware registered", "scope", prefix, "count", len(middleware))
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
		Addr:              srv.options.HealthAddr,
		Handler:           srv.healthMux,
		ReadTimeout:       srv.options.ReadTimeout,
		WriteTimeout:      srv.options.WriteTimeout,
		IdleTimeout:       srv.options.IdleTimeout,
		ReadHeaderTimeout: srv.options.ReadHeaderTimeout, // Prevent Slowloris attacks
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
	ln, err := net.Listen("tcp", srv.options.HealthAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", srv.options.HealthAddr, err)
	}
	srv.healthListener = ln

	go func() {
		srv.logger.Debug("Starting health server", "addr", srv.options.HealthAddr)
		if err := srv.healthServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srv.logger.Error("Health server encountered an error", "error", err)
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
			srv.logger.Info("Shutting down server.", "reason", context.Cause(triggerCtx))
			return srv.shutdownAfter(nil)
		case err := <-deferredErr:
			if err == nil {
				continue
			}
			srv.logger.Error("Deferred initialization failed", "error", err)
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
// died. It rejoins the deferred-init error chain when present.
func (srv *Server) handleServerExit(err error) error {
	srv.isRunning.Store(false)
	srv.isReady.Store(false)
	if srv.deferred.cancel != nil {
		srv.deferred.cancel()
	}
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
		srv.logger.Info("Deferred initialization started")
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
		srv.logger.Warn(message, "error", err)
	} else {
		srv.logger.Error(message, "error", err)
	}

	srv.setDeferredInitError(wrapped)
	srv.isReady.Store(false)

	shouldStop := srv.options.StopOnDeferredInitFailure
	if errors.Is(err, context.Canceled) {
		// If context was cancelled because the server is shutting down, always unblock Run.
		shouldStop = true
	}

	if !shouldStop {
		srv.logger.Warn("Deferred initialization failure will keep server in initializing state", "ready", false)
	}

	if shouldStop && errChan != nil {
		select {
		case errChan <- wrapped:
		default:
		}
	}
}

func (srv *Server) runOnReadyHooks(ctx context.Context) error {
	if len(srv.options.OnReadyHooks) == 0 {
		return nil
	}

	hookCtx := ctx
	if hookCtx == nil {
		hookCtx = context.Background()
	}

	srv.logger.Info("Executing OnReady hooks", "count", len(srv.options.OnReadyHooks))
	for i, hook := range srv.options.OnReadyHooks {
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
	srv.logger.Info("OnReady hooks completed")
	return nil
}

func (srv *Server) runOnReadyOnce(ctx context.Context) error {
	if len(srv.options.OnReadyHooks) == 0 {
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
	srv.logger.Info("Deferred initialization completed; server is ready")
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
			srv.logger.Error("Error during MCP handler shutdown.", "error", err)
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
	if len(srv.options.OnShutdownHooks) > 0 {
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

		srv.logger.Info("Executing shutdown hooks", "count", len(srv.options.OnShutdownHooks))
		for i, hook := range srv.options.OnShutdownHooks {
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
					srv.logger.Error("Shutdown hook error", "hook", i, "error", err)
				} else {
					srv.logger.Debug("Shutdown hook completed", "hook", i)
				}
			case <-hookCtx.Done():
				srv.logger.Warn("Shutdown hook timeout", "hook", i)
				// Continue with remaining hooks even if one times out
			}
		}
		srv.logger.Info("All shutdown hooks executed")
	}

	// Create an error channel to collect errors from goroutines
	errChan := make(chan error, 4)
	var wg sync.WaitGroup

	// Shutdown health server if it's running
	if srv.options.RunHealthServer && srv.healthServer != nil {
		wg.Go(func() {
			srv.logger.Info("Shutting down health server.")
			if err := srv.healthServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				srv.logger.Error("Error during health server shutdown.", "error", err)
				errChan <- fmt.Errorf("health server shutdown error: %w", err)
			}
			// Shutdown only knows about listeners after Serve has registered them.
			// Close our owned listener as a fallback for cancellation during the
			// narrow handoff between net.Listen and Serve.
			if srv.healthListener != nil {
				if err := srv.healthListener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
					errChan <- fmt.Errorf("health listener close error: %w", err)
				}
			}
		})
	}

	// Shutdown http server
	if srv.httpServer != nil {
		wg.Go(func() {
			srv.logger.Info("Shutting down http server.")
			if err := srv.httpServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				srv.logger.Error("Error during main server shutdown.", "error", err)
				errChan <- fmt.Errorf("main server shutdown error: %w", err)
			}
			if srv.listener != nil {
				if err := srv.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
					errChan <- fmt.Errorf("main listener close error: %w", err)
				}
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

	// Close os.Root handles if they exist
	if srv.staticRoot != nil {
		if err := srv.staticRoot.Close(); err != nil {
			srv.logger.Error("Failed to close static root", "error", err)
		}
	}
	if srv.templateRoot != nil {
		if err := srv.templateRoot.Close(); err != nil {
			srv.logger.Error("Failed to close template root", "error", err)
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

// Shutdown gracefully stops the server within the caller's deadline.
func (srv *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("hyperserve: Shutdown called with nil context")
	}
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

// Handle registers an http.Handler with the server's ServeMux and records
// the pattern for route inspection. Use HandleFunc for handler functions.
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

// Handler returns an ordinary http.Handler. Middleware registration remains
// open until the handler serves its first request, then its compiled plan and
// the server's middleware configuration are frozen.
func (srv *Server) Handler() http.Handler {
	return srv.middleware.applyToMux(srv.mux)
}

// Options returns an independent snapshot of the server configuration.
// Mutating the returned value does not reconfigure the running server.
func (srv *Server) Options() Options {
	return cloneOptions(srv.options)
}

// HandleFunc registers a handler function using [http.ServeMux] patterns.
// Requests pass through global middleware and matching UsePrefix middleware.
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

// =============================================================================
// Accessors for the builtin MCP tools/resources package
// =============================================================================
//
// mcp/builtin lives outside this package but needs read access to server
// runtime state to surface it via MCP. These accessors keep the
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

// SetMetrics overrides the request count and cumulative response time in microseconds.
//
// Deprecated: metrics are owned by request middleware. Tests should exercise
// requests through Handler instead of overwriting server counters.
func (srv *Server) SetMetrics(totalRequests uint64, totalResponseTime int64) {
	srv.totalRequests.Store(totalRequests)
	srv.totalResponseTime.Store(totalResponseTime)
}

// AddMetrics adds to the request count and cumulative response time in microseconds.
//
// Deprecated: metrics are owned by request middleware. Tests should exercise
// requests through Handler instead of updating server counters.
func (srv *Server) AddMetrics(deltaRequests uint64, deltaResponseTime int64) {
	srv.totalRequests.Add(deltaRequests)
	srv.totalResponseTime.Add(deltaResponseTime)
}

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
// mapping. The map and its stacks are independent snapshots.
func (srv *Server) MiddlewareRoutes() map[string]MiddlewareStack {
	if srv.middleware == nil {
		return nil
	}
	out := make(map[string]MiddlewareStack, len(srv.middleware.middleware))
	for route, stack := range srv.middleware.middleware {
		out[route] = slices.Clone(stack)
	}
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
	if srv.options.BannerColor {
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
	addr := srv.options.Addr
	if srv.options.EnableTLS {
		addr = srv.options.TLSAddr
	}

	protocol := "http"
	if srv.options.EnableTLS {
		protocol = "https"
	}

	// Print consolidated startup info
	fmt.Printf("\nServer:    %s://%s\n", protocol, addr)

	if srv.options.RunHealthServer {
		fmt.Printf("Health:    http://%s\n", srv.options.HealthAddr)
	}

	if srv.options.MCPEnabled {
		fmt.Printf("MCP:       %s (unified HTTP/SSE endpoint)\n", srv.options.MCPEndpoint)
		if srv.options.mcpTransportOpts.DeveloperMode {
			// Make MCP more discoverable for AI assistants
			fmt.Printf("\n🤖 MCP ENABLED: AI assistants connect via unified endpoint:\n")
			fmt.Printf("   SSE: GET %s://%s%s with Accept: text/event-stream\n", protocol, addr, srv.options.MCPEndpoint)
			fmt.Printf("   HTTP: POST %s://%s%s with Content-Type: application/json\n", protocol, addr, srv.options.MCPEndpoint)
		}
	}

	fmt.Println() // Empty line after banner
}
