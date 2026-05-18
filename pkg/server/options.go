/*
Package hyperserve provides configuration options for the HTTP server.

Configuration follows a hierarchical priority:
 1. Function parameters (highest priority)
 2. Environment variables
 3. Configuration file (options.json)
 4. Default values (lowest priority)

Environment Variables:
  - SERVER_ADDR: Main server address (default ":8080")
  - HEALTH_ADDR: Health check server address (default ":8081")
  - HS_HARDENED_MODE: Enable security headers (default "false")
  - HS_MCP_ENABLED: Enable Model Context Protocol (default "false")
  - HS_MCP_ENDPOINT: MCP endpoint path (default "/mcp")
  - HS_MCP_DEV: Enable MCP developer tools (default "false")
  - HS_MCP_OBSERVABILITY: Enable MCP observability resources (default "false")
  - HS_MCP_TRANSPORT: MCP transport type: "http" or "stdio" (default "http")
  - HS_CSP_WEB_WORKER_SUPPORT: Enable Web Worker CSP headers (default "false")
  - HS_LOG_LEVEL: Set log level (DEBUG, INFO, WARN, ERROR) (default "INFO")
  - HS_DEBUG: Enable debug mode and debug logging (default "false")
  - HS_SUPPRESS_BANNER: Suppress the HyperServe ASCII banner at startup (default "false")

Example configuration file (options.json):

	{
	  "addr": ":3000",
	  "tls": true,
	  "cert_file": "server.crt",
	  "key_file": "server.key",
	  "run_health_server": true,
	  "hardened_mode": true,
	  "debug_mode": false,
	  "log_level": "INFO"
	}
*/
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/osauer/hyperserve/pkg/mcp"
)

// ServerOptions contains all configuration settings for the HTTP server.
// Options can be set via WithXXX functions when creating a new server,
// environment variables, or a configuration file.
//
// Zero values are sensible defaults for most applications.
type ServerOptions struct {
	Addr                   string        `json:"addr,omitempty"`
	EnableTLS              bool          `json:"tls,omitempty"`
	TLSAddr                string        `json:"tls_addr,omitempty"`
	TLSHealthAddr          string        `json:"tls_health_addr,omitempty"`
	KeyFile                string        `json:"key_file,omitempty"`
	CertFile               string        `json:"cert_file,omitempty"`
	HealthAddr             string        `json:"health_addr,omitempty"`
	RateLimit              RateLimit     `json:"rate_limit,omitempty"`
	Burst                  int           `json:"burst,omitempty"`
	ReadTimeout            time.Duration `json:"read_timeout,omitempty"`
	WriteTimeout           time.Duration `json:"write_timeout,omitempty"`
	IdleTimeout            time.Duration `json:"idle_timeout,omitempty"`
	ReadHeaderTimeout      time.Duration `json:"read_header_timeout,omitempty"`
	StaticDir              string        `json:"static_dir,omitempty"`
	TemplateDir            string        `json:"template_dir,omitempty"`
	RunHealthServer        bool          `json:"run_health_server,omitempty"`
	AuthTokenValidatorFunc func(token string) (bool, error)
	FIPSMode               bool `json:"fips_mode,omitempty"`
	HardenedMode           bool `json:"hardened_mode,omitempty"`
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
	MCPDev              bool                                        `json:"mcp_dev,omitempty"`
	MCPObservability    bool                                        `json:"mcp_observability,omitempty"`
	MCPDiscoveryPolicy  mcp.DiscoveryPolicy                         `json:"mcp_discovery_policy,omitempty"`
	MCPDiscoveryFilter  func(toolName string, r *http.Request) bool `json:"-"` // Custom filter function
	mcpTransportOpts    mcp.TransportOptions                        // Internal transport options
	// CSP (Content Security Policy) configuration
	CSPWebWorkerSupport bool         `json:"csp_web_worker_support,omitempty"`
	CORS                *CORSOptions `json:"cors,omitempty"`
	// Logging configuration
	LogLevel  string `json:"log_level,omitempty"`
	DebugMode bool   `json:"debug_mode,omitempty"`
	// Banner configuration
	SuppressBanner bool `json:"suppress_banner,omitempty"`
	BannerColor    bool `json:"banner_color,omitempty"`

	// OnShutdownHooks are functions called when the server receives a shutdown signal.
	// Hooks are executed sequentially in the order they were added, before HTTP server shutdown.
	// Each hook receives a context with timeout and should respect the deadline.
	// Errors from hooks are logged but don't prevent shutdown.
	OnShutdownHooks []func(context.Context) error `json:"-"`

	// OnReadyHooks run after deferred initialization succeeds and before the server is marked ready.
	OnReadyHooks []func(context.Context, *Server) error `json:"-"`
	// StopOnDeferredInitFailure indicates whether the server should shut down if deferred init fails.
	StopOnDeferredInitFailure bool `json:"stop_on_deferred_init_failure,omitempty"`
}

var defaultServerOptions = &ServerOptions{
	Addr:                   ":8080",
	TLSAddr:                ":8443",
	HealthAddr:             ":9080",
	TLSHealthAddr:          ":9443",
	EnableTLS:              false,
	KeyFile:                "server.key",
	CertFile:               "server.crt",
	RateLimit:              1,
	Burst:                  10,
	ReadTimeout:            30 * time.Second, // Increased from 5s for better compatibility
	WriteTimeout:           30 * time.Second, // Increased from 10s for better compatibility
	IdleTimeout:            120 * time.Second,
	ReadHeaderTimeout:      10 * time.Second, // Slowloris protection
	StaticDir:              "static/",
	TemplateDir:            "template/",
	RunHealthServer:        false,
	AuthTokenValidatorFunc: func(token string) (bool, error) { return false, nil },
	FIPSMode:               false,
	HardenedMode:           false,
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
	MCPDev:              false, // Disabled by default - security sensitive
	MCPObservability:    false, // Disabled by default - users must opt-in
	// CSP defaults
	CSPWebWorkerSupport: false, // Disabled by default - users must opt-in
	// Logging defaults
	LogLevel:  "INFO",
	DebugMode: false,
	// Banner defaults
	SuppressBanner: false,
	BannerColor:    false,
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

// NewServerOptions creates a new ServerOptions instance with values loaded in priority order:
// 1. Environment variables (highest priority)
// 2. Configuration file (options.json)
// 3. Default values (lowest priority)
// Returns a fully initialized ServerOptions struct ready for use.
func NewServerOptions() *ServerOptions {
	// Create a copy of defaultServerOptions to avoid modifying the shared instance
	config := *defaultServerOptions
	configPtr := applyEnvVars(applyConfigFile(&config))
	configPtr.CORS = normalizeCORSOptions(configPtr.CORS)
	return configPtr
}

// ServerOptionFunc is a function type used to configure Server instances.
// It follows the functional options pattern for flexible server configuration.
type ServerOptionFunc func(srv *Server) error

// envBinding maps one HS_ environment variable to one field-write closure.
// The previous implementation unrolled this as ~180 lines of cut-and-paste
// branches; a single table is easier to scan and to extend.
type envBinding struct {
	name  string
	apply func(value string, c *ServerOptions)
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

func envBool(setter func(*ServerOptions, bool)) func(string, *ServerOptions) {
	return func(v string, c *ServerOptions) {
		if set, b := parseEnvBool(v); set {
			setter(c, b)
		}
	}
}

func defaultEnvBindings() []envBinding {
	return []envBinding{
		// String fields — assign verbatim when non-empty.
		{paramServerAddr, func(v string, c *ServerOptions) { c.Addr = v }},
		{paramHealthAddr, func(v string, c *ServerOptions) { c.HealthAddr = v }},
		{paramMCPEndpoint, func(v string, c *ServerOptions) { c.MCPEndpoint = v }},
		{paramMCPServerName, func(v string, c *ServerOptions) { c.MCPServerName = v }},
		{paramMCPServerVersion, func(v string, c *ServerOptions) { c.MCPServerVersion = v }},
		{paramMCPFileToolRoot, func(v string, c *ServerOptions) { c.MCPFileToolRoot = v }},
		{paramLogLevel, func(v string, c *ServerOptions) { c.LogLevel = v }},

		// Bool fields — only honour known truthy/falsy spellings.
		{paramHardenedMode, envBool(func(c *ServerOptions, b bool) {
			if b {
				c.HardenedMode = true
			}
		})},
		{paramMCPEnabled, envBool(func(c *ServerOptions, b bool) { c.MCPEnabled = b })},
		{paramMCPToolsEnabled, envBool(func(c *ServerOptions, b bool) { c.MCPToolsEnabled = b })},
		{paramMCPResourcesEnabled, envBool(func(c *ServerOptions, b bool) { c.MCPResourcesEnabled = b })},
		{paramMCPDev, envBool(func(c *ServerOptions, b bool) { c.MCPDev = b })},
		{paramMCPObservability, envBool(func(c *ServerOptions, b bool) { c.MCPObservability = b })},
		{paramCSPWebWorkerSupport, envBool(func(c *ServerOptions, b bool) { c.CSPWebWorkerSupport = b })},
		{paramSuppressBanner, envBool(func(c *ServerOptions, b bool) { c.SuppressBanner = b })},
		{paramBannerColor, envBool(func(c *ServerOptions, b bool) { c.BannerColor = b })},

		// Debug mode is a bool with a side effect (forces LogLevel=DEBUG)
		// so it doesn't fit the simple bool binding.
		{paramDebugMode, func(v string, c *ServerOptions) {
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
		{paramMCPTransport, func(v string, c *ServerOptions) {
			switch v {
			case "stdio":
				c.MCPTransport = mcp.StdioTransport
			case "http":
				c.MCPTransport = mcp.HTTPTransport
			}
		}},
	}
}

// applyEnvVars reads HS_-prefixed environment variables onto `config`.
// Bindings are table-driven (defaultEnvBindings); CORS variables stay
// inline below because each one feeds a different field on the nested
// CORSOptions struct and needs the same "lazy allocate, normalise once"
// post-pass.
func applyEnvVars(config *ServerOptions) *ServerOptions {
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

func ensureCORSOptions(config *ServerOptions) *CORSOptions {
	if config.CORS == nil {
		config.CORS = &CORSOptions{}
	}
	return config.CORS
}

// helper to read a options file and apply it to the options
func applyConfigFile(config *ServerOptions) *ServerOptions {
	file, err := os.Open(paramFileName)
	if err != nil {
		logger.Debug("Failed to open options file.", "error", err)
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
	fileConfig := &ServerOptions{}
	if err := decoder.Decode(fileConfig); err != nil {
		logger.Debug("No options file or loading failed; Using environment and defaults")
		return config
	}
	logger.Debug("Server configuration loaded from file", "file", paramFileName)
	mergeConfig(config, fileConfig)
	return config
}

// mergeConfig overrides default options with values of override if set
// Uses reflection to automatically merge all fields, eliminating the need for manual field copying
func mergeConfig(base *ServerOptions, override *ServerOptions) {
	baseValue := reflect.ValueOf(base).Elem()
	overrideValue := reflect.ValueOf(override).Elem()
	baseType := baseValue.Type()

	for i := range baseValue.NumField() {
		field := baseType.Field(i)
		baseField := baseValue.Field(i)
		overrideField := overrideValue.Field(i)

		// Skip non-exported fields or function fields (like AuthTokenValidatorFunc)
		if !baseField.CanSet() || field.Type.Kind() == reflect.Func {
			continue
		}

		// Check if override field is not zero value
		if !overrideField.IsZero() {
			baseField.Set(overrideField)
		}
	}
}

// setTimeouts applies non-zero timeouts to ServerOptions. StartServer reads
// these when it constructs the underlying *http.Server, so writing to the
// http.Server directly here would NPE — it is built lazily.
func (srv *Server) setTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) {
	if readTimeout != 0 {
		srv.Options.ReadTimeout = readTimeout
	}
	if writeTimeout != 0 {
		srv.Options.WriteTimeout = writeTimeout
	}
	if idleTimeout != 0 {
		srv.Options.IdleTimeout = idleTimeout
	}
}
