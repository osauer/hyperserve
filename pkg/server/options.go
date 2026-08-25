package server

/*
Configuration options for the HTTP server.

NewServer starts from deterministic defaults. Configuration sources and
functional options are applied left to right, so later options win:

	srv, err := NewServer(
		WithConfigFile("options.json"),
		WithEnvironment(),
		WithAddr(":3000"),
	)

Configuration files and environment variables are never read unless their
option is passed.

WithEnvironment reads these variables:
  - SERVER_ADDR, HS_PORT: Main server address, or a port shortcut such as "8080"
  - HEALTH_ADDR: Health-check server address
  - HS_RATE_LIMIT, HS_BURST_LIMIT: Per-client rate and burst limits
  - HS_SERVER_HEADER: Identification emitted by HeadersMiddleware
  - HS_MCP_ENABLED, HS_MCP_ENDPOINT: MCP enablement and HTTP endpoint
  - HS_MCP_SERVER_NAME, HS_MCP_SERVER_VERSION: MCP server identity
  - HS_MCP_TOOLS_ENABLED, HS_MCP_RESOURCES_ENABLED: Built-in MCP capabilities
  - HS_MCP_FILE_TOOL_ROOT: Root available to built-in MCP file tools
  - HS_MCP_DEV, HS_MCP_OBSERVABILITY: Built-in development and observability features
  - HS_MCP_TRANSPORT: "http" or "stdio"
  - HS_MCP_PROTOCOL_VERSION: MCP protocol version to advertise
  - HS_CSP_WEB_WORKER_SUPPORT: Web Worker CSP allowance
  - HS_CORS_ALLOWED_ORIGINS, HS_CORS_ALLOW_CREDENTIALS: CORS origins and credentials
  - HS_CORS_ALLOWED_METHODS, HS_CORS_ALLOWED_HEADERS: CORS request policy
  - HS_CORS_EXPOSE_HEADERS, HS_CORS_MAX_AGE: CORS response policy and cache duration
  - HS_LOG_LEVEL, HS_DEBUG: Log verbosity and debug mode
  - HS_STARTUP_BANNER, HS_BANNER_COLOR: Startup banner visibility and color

WithEnvironment does not choose a configuration file. Explicit composition
passes the chosen path to WithConfigFile. Static and template roots have no
environment bindings; applications must grant those filesystem capabilities
explicitly.

Example configuration file (options.json):

	{
	  "addr": ":3000",
	  "tls": true,
	  "cert_file": "server.crt",
	  "key_file": "server.key",
	  "run_health_server": true,
	  "server_header": "example-service",
	  "debug_mode": false,
	  "log_level": "INFO"
	}
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/osauer/hyperserve/v2/pkg/mcp"
)

// Options contains all configuration settings for the HTTP server.
// Values can be set via WithXXX functions when creating a new server. Bind a
// configuration file or environment variables explicitly with WithConfigFile
// or WithEnvironment.
//
// Use DefaultOptions to obtain HyperServe's defaults before modifying a
// complete snapshot; a zero Options value is not implicitly filled.
type Options struct {
	Addr              string        `json:"addr,omitempty"`
	EnableTLS         bool          `json:"tls,omitempty"`
	TLSAddr           string        `json:"tls_addr,omitempty"`
	KeyFile           string        `json:"key_file,omitempty"`
	CertFile          string        `json:"cert_file,omitempty"`
	HealthAddr        string        `json:"health_addr,omitempty"`
	RateLimit         RateLimit     `json:"rate_limit,omitempty"`
	Burst             int           `json:"burst,omitempty"`
	ReadTimeout       time.Duration `json:"read_timeout,omitempty"`
	WriteTimeout      time.Duration `json:"write_timeout,omitempty"`
	IdleTimeout       time.Duration `json:"idle_timeout,omitempty"`
	ReadHeaderTimeout time.Duration `json:"read_header_timeout,omitempty"`
	StaticDir         string        `json:"static_dir,omitempty"`
	TemplateDir       string        `json:"template_dir,omitempty"`
	RunHealthServer   bool          `json:"run_health_server,omitempty"`
	FIPSMode          bool          `json:"fips_mode,omitempty"`
	// ServerHeader is emitted by HeadersMiddleware when non-empty.
	ServerHeader string `json:"server_header,omitempty"`
	// MCP (Model Context Protocol) configuration
	MCPEnabled          bool                                        `json:"mcp_enabled,omitempty"`
	MCPEndpoint         string                                      `json:"mcp_endpoint,omitempty"`
	MCPServerName       string                                      `json:"mcp_server_name,omitempty"`
	MCPServerVersion    string                                      `json:"mcp_server_version,omitempty"`
	MCPToolsEnabled     bool                                        `json:"mcp_tools_enabled,omitempty"`
	MCPResourcesEnabled bool                                        `json:"mcp_resources_enabled,omitempty"`
	MCPFileToolRoot     string                                      `json:"mcp_file_tool_root,omitempty"`
	MCPLogResourceSize  int                                         `json:"mcp_log_resource_size,omitempty"`
	MCPToolCallTimeout  time.Duration                               `json:"mcp_tool_call_timeout,omitempty"`
	MCPTransport        mcp.TransportType                           `json:"mcp_transport,omitempty"`
	MCPProtocolVersion  string                                      `json:"mcp_protocol_version,omitempty"`
	MCPLegacyRoutedSSE  bool                                        `json:"mcp_legacy_routed_sse,omitempty"`
	MCPDev              bool                                        `json:"mcp_dev,omitempty"`
	MCPObservability    bool                                        `json:"mcp_observability,omitempty"`
	MCPDiscoveryPolicy  mcp.DiscoveryPolicy                         `json:"mcp_discovery_policy,omitempty"`
	MCPDiscoveryFilter  func(toolName string, r *http.Request) bool `json:"-"` // Custom filter function
	MCPOriginValidator  func(r *http.Request) bool                  `json:"-"`
	mcpTransportOpts    mcp.TransportOptions                        // Internal transport options
	// CSP (Content Security Policy) configuration
	CSPWebWorkerSupport bool         `json:"csp_web_worker_support,omitempty"`
	CORS                *CORSOptions `json:"cors,omitempty"`
	// Logging configuration
	LogLevel  string `json:"log_level,omitempty"`
	DebugMode bool   `json:"debug_mode,omitempty"`
	// Banner configuration
	StartupBanner bool `json:"startup_banner,omitempty"`
	BannerColor   bool `json:"banner_color,omitempty"`

	// OnShutdownHooks are functions called when the server begins shutdown.
	// Hooks are executed sequentially in the order they were added, before HTTP server shutdown.
	// Each hook receives a context with timeout and should respect the deadline.
	// Errors from hooks are logged but don't prevent shutdown.
	OnShutdownHooks []func(context.Context) error `json:"-"`

	// OnReadyHooks run after deferred initialization succeeds and before the server is marked ready.
	OnReadyHooks []func(context.Context, *Server) error `json:"-"`
	// StopOnDeferredInitFailure indicates whether the server should shut down if deferred init fails.
	StopOnDeferredInitFailure bool `json:"stop_on_deferred_init_failure,omitempty"`
}

var defaultOptions = Options{
	Addr:              ":8080",
	TLSAddr:           ":8443",
	HealthAddr:        ":9080",
	EnableTLS:         false,
	KeyFile:           "server.key",
	CertFile:          "server.crt",
	RateLimit:         1,
	Burst:             10,
	ReadTimeout:       30 * time.Second, // Increased from 5s for better compatibility
	WriteTimeout:      30 * time.Second, // Increased from 10s for better compatibility
	IdleTimeout:       120 * time.Second,
	ReadHeaderTimeout: 10 * time.Second, // Slowloris protection
	// Filesystem capabilities are opt-in. A library default must not turn the
	// embedding process's working directory into an application asset root.
	StaticDir:       "",
	TemplateDir:     "",
	RunHealthServer: false,
	FIPSMode:        false,
	// MCP defaults
	MCPEnabled:          false,
	MCPEndpoint:         "/mcp",
	MCPServerName:       "hyperserve",
	MCPServerVersion:    "1.0.0",
	MCPToolsEnabled:     false, // Disabled by default - users must opt-in
	MCPResourcesEnabled: false, // Disabled by default - users must opt-in
	MCPFileToolRoot:     "",
	MCPLogResourceSize:  100,
	MCPToolCallTimeout:  30 * time.Second,
	MCPTransport:        mcp.HTTPTransport,
	MCPProtocolVersion:  mcp.DefaultProtocolVersion,
	MCPDev:              false, // Disabled by default - security sensitive
	MCPObservability:    false, // Disabled by default - users must opt-in
	// CSP defaults
	CSPWebWorkerSupport: false, // Disabled by default - users must opt-in
	// Logging defaults
	LogLevel:  "INFO",
	DebugMode: false,
	// Identification and banner output are opt-in for library consumers.
	ServerHeader:  "",
	StartupBanner: false,
	BannerColor:   false,
	// Deferred init defaults
	StopOnDeferredInitFailure: true,
}

// Log level constants for server configuration.
// These wrap slog levels to provide a consistent API while hiding the logging implementation details.
const (
	// LevelDebug enables debug-level logging with detailed information
	LevelDebug = slog.LevelDebug
	// LevelInfo enables info-level logging for general information
	LevelInfo = slog.LevelInfo
	// LevelWarn enables warning-level logging for important but non-critical events
	LevelWarn = slog.LevelWarn
	// LevelError enables error-level logging for error conditions only
	LevelError = slog.LevelError
)

// DefaultOptions returns an independent copy of HyperServe's deterministic
// defaults. Nested slices and CORS configuration are cloned so callers may
// safely modify the result before passing it to [WithOptions].
func DefaultOptions() Options {
	return cloneOptions(defaultOptions)
}

func cloneOptions(options Options) Options {
	clone := options
	clone.CORS = normalizeCORSOptions(options.CORS)
	clone.OnShutdownHooks = slices.Clone(options.OnShutdownHooks)
	clone.OnReadyHooks = slices.Clone(options.OnReadyHooks)
	return clone
}

func normalizeOptions(options *Options) error {
	options.CORS = normalizeCORSOptions(options.CORS)
	for i := 0; i < len(options.ServerHeader); i++ {
		b := options.ServerHeader[i]
		if b == '\r' || b == '\n' || b == 0x7f || (b < 0x20 && b != '\t') {
			return fmt.Errorf("server header contains invalid control byte 0x%02x", b)
		}
	}
	return nil
}

// Option configures a Server during construction.
type Option func(*Server) error

// envBinding maps one HS_ environment variable to one field-write closure.
// The previous implementation unrolled this as ~180 lines of cut-and-paste
// branches; a single table is easier to scan and to extend.
type envBinding struct {
	name  string
	apply func(value string, c *Options)
}

// parseEnvBool reports whether `v` looks like a "true"-ish value. Returns
// (set=false) when the input doesn't match any known form, leaving the
// caller's field untouched. Accepts the canonical pair plus yes/no/on/off
// because BannerColor historically accepted them and reusing this helper
// avoids two parsers.
func parseEnvBool(v string) (set, value bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return true, false
	}
	return false, false
}

func envBool(setter func(*Options, bool)) func(string, *Options) {
	return func(v string, c *Options) {
		if set, b := parseEnvBool(v); set {
			setter(c, b)
		}
	}
}

func defaultEnvBindings() []envBinding {
	return []envBinding{
		// String fields — assign verbatim when non-empty.
		{paramServerAddr, func(v string, c *Options) { c.Addr = v }},
		{paramServerPort, func(v string, c *Options) {
			port := strings.TrimSpace(v)
			if port == "" {
				return
			}
			if strings.HasPrefix(port, ":") {
				c.Addr = port
				return
			}
			c.Addr = ":" + port
		}},
		{paramHealthAddr, func(v string, c *Options) { c.HealthAddr = v }},
		{paramMCPEndpoint, func(v string, c *Options) { c.MCPEndpoint = v }},
		{paramMCPServerName, func(v string, c *Options) { c.MCPServerName = v }},
		{paramMCPServerVersion, func(v string, c *Options) { c.MCPServerVersion = v }},
		{paramMCPFileToolRoot, func(v string, c *Options) { c.MCPFileToolRoot = v }},
		{paramMCPProtocolVersion, func(v string, c *Options) { c.MCPProtocolVersion = v }},
		{paramLogLevel, func(v string, c *Options) { c.LogLevel = v }},
		{paramServerHeader, func(v string, c *Options) { c.ServerHeader = v }},

		// Bool fields — only honour known truthy/falsy spellings.
		{paramMCPEnabled, envBool(func(c *Options, b bool) { c.MCPEnabled = b })},
		{paramMCPToolsEnabled, envBool(func(c *Options, b bool) { c.MCPToolsEnabled = b })},
		{paramMCPResourcesEnabled, envBool(func(c *Options, b bool) { c.MCPResourcesEnabled = b })},
		{paramMCPDev, envBool(func(c *Options, b bool) { c.MCPDev = b })},
		{paramMCPObservability, envBool(func(c *Options, b bool) { c.MCPObservability = b })},
		{paramCSPWebWorkerSupport, envBool(func(c *Options, b bool) { c.CSPWebWorkerSupport = b })},
		{paramStartupBanner, envBool(func(c *Options, b bool) { c.StartupBanner = b })},
		{paramBannerColor, envBool(func(c *Options, b bool) { c.BannerColor = b })},

		// Debug mode is a bool with a side effect (forces LogLevel=DEBUG)
		// so it doesn't fit the simple bool binding.
		{paramDebugMode, func(v string, c *Options) {
			set, b := parseEnvBool(v)
			if !set {
				return
			}
			c.DebugMode = b
			if b {
				c.LogLevel = "DEBUG"
			}
		}},

		// MCP transport is an enum decoded into mcp.TransportType.
		{paramMCPTransport, func(v string, c *Options) {
			switch v {
			case "stdio":
				c.MCPTransport = mcp.StdioTransport
			case "http":
				c.MCPTransport = mcp.HTTPTransport
			}
		}},

		// Rate-limit fields.
		{paramRateLimit, func(v string, c *Options) {
			if rateLimit, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && rateLimit >= 0 {
				c.RateLimit = RateLimit(rateLimit)
			}
		}},
		{paramBurstLimit, func(v string, c *Options) {
			if burst, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && burst >= 0 {
				c.Burst = burst
			}
		}},
	}
}

// applyEnvVars reads HS_-prefixed environment variables onto `config`.
// Bindings are table-driven (defaultEnvBindings); CORS variables stay
// inline below because each one feeds a different field on the nested
// CORSOptions struct and needs the same "lazy allocate, normalise once"
// post-pass.
func applyEnvVars(config *Options) *Options {
	for _, b := range defaultEnvBindings() {
		if v := os.Getenv(b.name); v != "" {
			b.apply(v, config)
		}
	}

	// CORS environment variables — each one mutates a nested struct that
	// must exist by the time we write to it, so the inline form stays.
	corsConfigured := false
	if allowed := os.Getenv(paramCORSAllowedOrigins); allowed != "" {
		ensureCORSOptions(config).AllowedOrigins = sanitizeTokens(strings.Split(allowed, ","), false)
		corsConfigured = true
	}
	if methods := os.Getenv(paramCORSAllowedMethods); methods != "" {
		ensureCORSOptions(config).AllowedMethods = sanitizeTokens(strings.Split(methods, ","), true)
		corsConfigured = true
	}
	if headers := os.Getenv(paramCORSAllowedHeaders); headers != "" {
		ensureCORSOptions(config).AllowedHeaders = sanitizeTokens(strings.Split(headers, ","), false)
		corsConfigured = true
	}
	if expose := os.Getenv(paramCORSExposeHeaders); expose != "" {
		ensureCORSOptions(config).ExposeHeaders = sanitizeTokens(strings.Split(expose, ","), false)
		corsConfigured = true
	}
	if allowCreds := os.Getenv(paramCORSAllowCredentials); allowCreds != "" {
		if set, b := parseEnvBool(allowCreds); set {
			ensureCORSOptions(config).AllowCredentials = b
			corsConfigured = true
		}
	}
	if maxAge := os.Getenv(paramCORSMaxAge); maxAge != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(maxAge)); err == nil && seconds >= 0 {
			ensureCORSOptions(config).MaxAgeSeconds = seconds
			corsConfigured = true
		}
	}
	if corsConfigured {
		config.CORS = normalizeCORSOptions(config.CORS)
	}

	return config
}

func ensureCORSOptions(config *Options) *CORSOptions {
	if config.CORS == nil {
		config.CORS = &CORSOptions{}
	}
	return config.CORS
}

func loadConfigFile(config *Options, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawFields); err != nil {
		return fmt.Errorf("decode fields: %w", err)
	}
	if rawFields == nil {
		return errors.New("config must be a JSON object")
	}
	fileConfig := &Options{}
	if err := json.Unmarshal(data, fileConfig); err != nil {
		return fmt.Errorf("decode options: %w", err)
	}
	slog.Default().Debug("Server configuration loaded from file", "file", path)
	mergeConfig(config, fileConfig, rawFields)
	return nil
}

// mergeConfig overrides default options with fields present in the JSON file.
// Presence matters: false, 0, "", and null are intentional config values and
// must be able to override defaults.
func mergeConfig(base *Options, override *Options, rawFields map[string]json.RawMessage) {
	baseValue := reflect.ValueOf(base).Elem()
	overrideValue := reflect.ValueOf(override).Elem()
	baseType := baseValue.Type()

	for i := range baseValue.NumField() {
		field := baseType.Field(i)
		baseField := baseValue.Field(i)
		overrideField := overrideValue.Field(i)

		// Skip non-exported and function fields.
		if !baseField.CanSet() || field.Type.Kind() == reflect.Func {
			continue
		}

		name, skip := configJSONFieldName(field)
		if skip {
			continue
		}
		if _, present := rawFields[name]; present {
			baseField.Set(overrideField)
		}
	}
}

func configJSONFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	if name, _, ok := strings.Cut(tag, ","); ok {
		if name == "" {
			return field.Name, false
		}
		return name, false
	}
	if tag != "" {
		return tag, false
	}
	return field.Name, false
}

// setTimeouts records the exact timeout values. Zero deliberately disables a
// timeout, matching net/http.Server and allowing streaming responses.
func (srv *Server) setTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) {
	srv.options.ReadTimeout = readTimeout
	srv.options.WriteTimeout = writeTimeout
	srv.options.IdleTimeout = idleTimeout
}

// --- Option constructors ----------------------------------------
//

// WithOptions replaces the current option snapshot with a defensive copy of
// options. Options passed later to NewServer override this snapshot.
func WithOptions(options Options) Option {
	return func(srv *Server) error {
		clone := cloneOptions(options)
		srv.options = clone
		return nil
	}
}

// WithConfigFile overlays fields present in the JSON file at path. An explicit
// file is required to exist and contain one valid JSON object.
func WithConfigFile(path string) Option {
	return func(srv *Server) error {
		if strings.TrimSpace(path) == "" {
			return errors.New("config file path required")
		}
		if err := loadConfigFile(&srv.options, path); err != nil {
			return fmt.Errorf("load server config %q: %w", path, err)
		}
		return nil
	}
}

// WithEnvironment overlays supported SERVER_ADDR, HEALTH_ADDR, and HS_*
// variables. It does not consult HS_CONFIG_PATH; use WithConfigFile when the
// application chooses to read a file.
func WithEnvironment() Option {
	return func(srv *Server) error {
		applyEnvVars(&srv.options)
		return nil
	}
}

// These are the public `With*` knobs callers pass to `NewServer`. They live
// alongside `Options` (the struct they mutate) and the env-binding
// machinery above, instead of sharing space with the lifecycle code in
// `server.go`. The MCP-specific options are split out further into
// `options_mcp.go` and `options_mcp_discovery.go` for the same reason.

// WithTLS enables TLS on the server with the specified certificate and key files.
// Returns a Option that configures TLS settings and validates file existence.
func WithTLS(certFile, keyFile string) Option {
	return func(srv *Server) error {
		wd, _ := os.Getwd()
		// do not override existing values if not set
		if certFile != "" {
			srv.options.CertFile = certFile
		}
		if keyFile != "" {
			srv.options.KeyFile = keyFile
		}
		// check if the files exist
		errCert := checkfile(certFile, wd)
		errKey := checkfile(keyFile, wd)
		if errCert != nil || errKey != nil {
			errs := make([]error, 0, 2)
			if errCert != nil {
				errs = append(errs, fmt.Errorf("cert file %q: %w", certFile, errCert))
			}
			if errKey != nil {
				errs = append(errs, fmt.Errorf("key file %q: %w", keyFile, errKey))
			}
			return fmt.Errorf("error checking TLS files: %w", errors.Join(errs...))
		}
		srv.options.EnableTLS = true
		return nil
	}
}

// WithDebugMode enables debug logging and additional debug features.
// (The previously-exported WithLoglevel had no callers; use WithDebugMode
// or the HS_LOG_LEVEL env var to change the log level.)
func WithDebugMode() Option {
	return func(srv *Server) error {
		srv.options.DebugMode = true
		srv.options.LogLevel = "DEBUG"
		return nil
	}
}

// WithStartupBanner opts into HyperServe's ASCII startup banner. Library
// consumers are silent by default apart from configured structured logs.
func WithStartupBanner() Option {
	return func(srv *Server) error {
		srv.options.StartupBanner = true
		return nil
	}
}

// WithDeferredInit registers a callback that runs after the server listener is active but before
// the server is marked ready. While the callback is executing, non-health endpoints receive 503.
func WithDeferredInit(fn func(context.Context, *Server) error) Option {
	return func(srv *Server) error {
		srv.deferred.init = fn
		return nil
	}
}

// WithOnReady registers hooks that run after deferred initialization succeeds but before the server
// is marked ready. Hooks are executed sequentially in the order they were registered.
func WithOnReady(hook func(context.Context, *Server) error) Option {
	return func(srv *Server) error {
		if hook != nil {
			srv.options.OnReadyHooks = append(srv.options.OnReadyHooks, hook)
		}
		return nil
	}
}

// WithDeferredInitStopOnFailure configures whether the server should shut down if the deferred
// initialization callback returns an error. Defaults to true.
func WithDeferredInitStopOnFailure(stop bool) Option {
	return func(srv *Server) error {
		srv.options.StopOnDeferredInitFailure = stop
		return nil
	}
}

// WithOnShutdown registers a function to be called when the server begins shutdown.
// Multiple hooks can be registered and are executed sequentially in the order they were added.
// Hooks are called before the HTTP server shutdown begins, allowing applications to cleanly
// stop their own goroutines and release resources.
//
// Each hook receives a context with a timeout (typically 5 seconds of the total 10-second
// shutdown budget). Hooks should respect the context deadline and return promptly.
// Errors from hooks are logged but don't prevent shutdown from proceeding.
//
// Example:
//
//	srv, _ := server.NewServer(
//		server.WithOnShutdown(func(ctx context.Context) error {
//			log.Println("Stopping background workers...")
//			return stopWorkers(ctx)
//		}),
//	)
func WithOnShutdown(hook func(context.Context) error) Option {
	return func(srv *Server) error {
		if hook != nil {
			srv.options.OnShutdownHooks = append(srv.options.OnShutdownHooks, hook)
		}
		return nil
	}
}

// WithHealthServer enables the health server on a separate port.
// The health server provides /healthz/, /readyz/, and /livez/ endpoints for monitoring.
func WithHealthServer() Option {
	return func(srv *Server) error {
		srv.options.RunHealthServer = true
		return nil
	}
}

// WithHealthAddr sets the address for the separate health server.
func WithHealthAddr(addr string) Option {
	return func(srv *Server) error {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("invalid health address %q: %w", addr, err)
		}
		srv.options.HealthAddr = addr
		return nil
	}
}

// WithLogLevel sets the configured server log level. Accepted values are
// DEBUG, INFO, WARN, and ERROR.
func WithLogLevel(level string) Option {
	return func(srv *Server) error {
		srv.options.LogLevel = level
		return nil
	}
}

// WithAddr sets the address and port for the server to listen on.
// The address must be in the format "host:port" (e.g., ":8080", "localhost:3000").
func WithAddr(addr string) Option {
	return func(srv *Server) error {
		// validate the address
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("invalid server address %q: %w", addr, err)
		}
		srv.options.Addr = addr
		return nil
	}
}

// WithTimeouts configures the HTTP server timeouts.
// readTimeout: maximum duration for reading the entire request
// writeTimeout: maximum duration before timing out writes of the response
// idleTimeout: maximum time to wait for the next request when keep-alives are enabled
func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) Option {
	return func(srv *Server) error {
		srv.setTimeouts(readTimeout, writeTimeout, idleTimeout)
		return nil
	}
}

// WithRateLimit configures rate limiting for the server.
// limit: maximum number of requests per second per client IP
// burst: maximum number of requests that can be made in a short burst
func WithRateLimit(limit RateLimit, burst int) Option {
	return func(srv *Server) error {
		srv.options.RateLimit = limit
		srv.options.Burst = burst
		return nil
	}
}

// WithCORS configures Cross-Origin Resource Sharing options for HTTP handlers.
func WithCORS(opts *CORSOptions) Option {
	return func(srv *Server) error {
		srv.options.CORS = normalizeCORSOptions(opts)
		return nil
	}
}

// WithStaticDir sets the directory root used by [Server.HandleStatic].
// The directory is opened and validated when the static route is registered.
func WithStaticDir(dir string) Option {
	return func(srv *Server) error {
		srv.options.StaticDir = dir
		return nil
	}
}

// WithTemplateDir sets the directory path where HTML templates are located.
// Templates in this directory can be used with HandleTemplate and HandleFuncDynamic methods.
// Returns an error if the specified directory does not exist or is not accessible.
func WithTemplateDir(dir string) Option {
	return func(srv *Server) error {
		// Check if the directory exists and is accessible
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("template directory not found: %s", dir)
			}
			return fmt.Errorf("template directory access error: %s: %w", dir, err)
		}

		srv.options.TemplateDir = dir
		return nil
	}
}

// WithFIPSMode restricts the TLS handshake to FIPS-approved cipher suites
// and elliptic curves.
//
// This is NOT full FIPS 140-3 compliance:
//   - it does not switch the Go toolchain into FIPS mode (build with
//     GOFIPS140 for that);
//   - it does not constrain non-TLS crypto (hashes, RNGs, signatures
//     outside TLS);
//   - it does not invoke a FIPS-validated cryptographic module.
//
// Use this for "TLS handshake uses FIPS-approved primitives." For deployments
// that require true FIPS 140-3 compliance, combine with a FIPS-validated
// toolchain build.
func WithFIPSMode() Option {
	return func(srv *Server) error {
		srv.options.FIPSMode = true
		return nil
	}
}

// WithServerHeader opts into a Server response header when
// [HeadersMiddleware] is installed. The empty string omits identification.
// Invalid HTTP control bytes cause NewServer to return an error.
func WithServerHeader(value string) Option {
	return func(srv *Server) error {
		srv.options.ServerHeader = value
		return nil
	}
}

// WithCSPWebWorkerSupport enables Content Security Policy support for Web Workers using blob: URLs.
// This is required for modern web applications that use libraries like Tone.js, PDF.js, or other
// libraries that create Web Workers with blob: URLs for performance optimization.
// By default, this is disabled for security reasons and must be explicitly enabled.
func WithCSPWebWorkerSupport() Option {
	return func(srv *Server) error {
		srv.options.CSPWebWorkerSupport = true
		return nil
	}
}
