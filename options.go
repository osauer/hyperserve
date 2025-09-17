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
package hyperserve

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"time"
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
	RateLimit              rateLimit     `json:"rate_limit,omitempty"`
	Burst                  int           `json:"burst,omitempty"`
	ReadTimeout            time.Duration `json:"read_timeout,omitempty"`
	WriteTimeout           time.Duration `json:"write_timeout,omitempty"`
	IdleTimeout            time.Duration `json:"idle_timeout,omitempty"`
	ReadHeaderTimeout      time.Duration `json:"read_header_timeout,omitempty"`
	StaticDir              string        `json:"static_dir,omitempty"`
	TemplateDir            string        `json:"template_dir,omitempty"`
	RunHealthServer        bool          `json:"run_health_server,omitempty"`
	ChaosMode              bool          `json:"chaos_mode,omitempty"`
	ChaosMaxLatency        time.Duration `json:"chaos_max_latency,omitempty"`
	ChaosMinLatency        time.Duration `json:"chaos_min_latency,omitempty"`
	ChaosErrorRate         float64       `json:"chaos_error_rate,omitempty"`
	ChaosThrottleRate      float64       `json:"chaos_throttle_rate,omitempty"`
	ChaosPanicRate         float64       `json:"chaos_panic_rate,omitempty"`
	AuthTokenValidatorFunc func(token string) (bool, error)
	FIPSMode               bool     `json:"fips_mode,omitempty"`
	EnableECH              bool     `json:"enable_ech,omitempty"`
	ECHKeys                [][]byte `json:"-"` // ECH keys are sensitive, don't serialize
	HardenedMode           bool     `json:"hardened_mode,omitempty"`
	// MCP (Model Context Protocol) configuration
	MCPEnabled             bool     `json:"mcp_enabled,omitempty"`
	MCPEndpoint            string   `json:"mcp_endpoint,omitempty"`
	MCPServerName          string   `json:"mcp_server_name,omitempty"`
	MCPServerVersion       string   `json:"mcp_server_version,omitempty"`
	MCPToolsEnabled        bool     `json:"mcp_tools_enabled,omitempty"`
	MCPResourcesEnabled    bool     `json:"mcp_resources_enabled,omitempty"`
	MCPFileToolRoot        string   `json:"mcp_file_tool_root,omitempty"`
	MCPLogResourceSize     int      `json:"mcp_log_resource_size,omitempty"`
	MCPTransport           MCPTransportType `json:"mcp_transport,omitempty"`
	MCPDev                 bool     `json:"mcp_dev,omitempty"`
	MCPObservability       bool     `json:"mcp_observability,omitempty"`
	MCPDiscoveryPolicy     DiscoveryPolicy `json:"mcp_discovery_policy,omitempty"`
	MCPDiscoveryFilter     func(toolName string, r *http.Request) bool `json:"-"` // Custom filter function
	mcpTransportOpts       mcpTransportOptions // Internal transport options
	// CSP (Content Security Policy) configuration
	CSPWebWorkerSupport    bool     `json:"csp_web_worker_support,omitempty"`
	// Logging configuration
	LogLevel               string   `json:"log_level,omitempty"`
	DebugMode              bool     `json:"debug_mode,omitempty"`
	// Banner configuration
	SuppressBanner         bool     `json:"suppress_banner,omitempty"`

	// OnShutdownHooks are functions called when the server receives a shutdown signal.
	// Hooks are executed sequentially in the order they were added, before HTTP server shutdown.
	// Each hook receives a context with timeout and should respect the deadline.
	// Errors from hooks are logged but don't prevent shutdown.
	OnShutdownHooks        []func(context.Context) error `json:"-"`
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
	ReadTimeout:            30 * time.Second,    // Increased from 5s for better compatibility
	WriteTimeout:           30 * time.Second,    // Increased from 10s for better compatibility
	IdleTimeout:            120 * time.Second,
	ReadHeaderTimeout:      10 * time.Second,    // Slowloris protection
	StaticDir:              "static/",
	TemplateDir:            "template/",
	RunHealthServer:        false,
	ChaosMode:              false,
	ChaosMaxLatency:        2 * time.Second,
	ChaosMinLatency:        500 * time.Millisecond,
	ChaosErrorRate:         0.1,
	ChaosThrottleRate:      0.05,
	ChaosPanicRate:         0.01,
	AuthTokenValidatorFunc: func(token string) (bool, error) { return false, nil },
	FIPSMode:               false,
	EnableECH:              false,
	HardenedMode:           false,
	// MCP defaults
	MCPEnabled:             false,
	MCPEndpoint:            "/mcp",
	MCPServerName:          "hyperserve",
	MCPServerVersion:       "1.0.0",
	MCPToolsEnabled:        false,  // Disabled by default - users must opt-in
	MCPResourcesEnabled:    false,  // Disabled by default - users must opt-in
	MCPFileToolRoot:        "",
	MCPLogResourceSize:     100,
	MCPTransport:           HTTPTransport,
	MCPDev:                 false,  // Disabled by default - security sensitive
	MCPObservability:       false,  // Disabled by default - users must opt-in
	// CSP defaults
	CSPWebWorkerSupport:    false,  // Disabled by default - users must opt-in
	// Logging defaults
	LogLevel:               "INFO",
	DebugMode:              false,
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
	return configPtr
}

// ServerOptionFunc is a function type used to configure Server instances.
// It follows the functional options pattern for flexible server configuration.
type ServerOptionFunc func(srv *Server) error

// helper to read environment variables and apply them to the options
func applyEnvVars(config *ServerOptions) *ServerOptions {
	if addr := os.Getenv(paramServerAddr); addr != "" {
		config.Addr = addr
		logger.Debug("Server address set from environment variable", "variable", paramServerAddr, "addr", addr)
	}
	if healthAddr := os.Getenv(paramHealthAddr); healthAddr != "" {
		config.HealthAddr = healthAddr
		logger.Debug("Health endpoint address set from environment variable", "variable", paramHealthAddr, "addr", healthAddr)
	}
	if hardenedMode := os.Getenv(paramHardenedMode); hardenedMode != "" {
		if hardenedMode == "true" || hardenedMode == "1" {
			config.HardenedMode = true
			logger.Debug("Hardened mode enabled from environment variable", "variable", paramHardenedMode)
		}
	}
	
	// MCP (Model Context Protocol) environment variables
	if mcpEnabled := os.Getenv(paramMCPEnabled); mcpEnabled != "" {
		if mcpEnabled == "true" || mcpEnabled == "1" {
			config.MCPEnabled = true
			logger.Debug("MCP enabled from environment variable", "variable", paramMCPEnabled)
		} else if mcpEnabled == "false" || mcpEnabled == "0" {
			config.MCPEnabled = false
			logger.Debug("MCP disabled from environment variable", "variable", paramMCPEnabled)
		}
	}
	if mcpEndpoint := os.Getenv(paramMCPEndpoint); mcpEndpoint != "" {
		config.MCPEndpoint = mcpEndpoint
		logger.Debug("MCP endpoint set from environment variable", "variable", paramMCPEndpoint, "endpoint", mcpEndpoint)
	}
	if mcpServerName := os.Getenv(paramMCPServerName); mcpServerName != "" {
		config.MCPServerName = mcpServerName
		logger.Debug("MCP server name set from environment variable", "variable", paramMCPServerName, "name", mcpServerName)
	}
	if mcpServerVersion := os.Getenv(paramMCPServerVersion); mcpServerVersion != "" {
		config.MCPServerVersion = mcpServerVersion
		logger.Debug("MCP server version set from environment variable", "variable", paramMCPServerVersion, "version", mcpServerVersion)
	}
	if mcpToolsEnabled := os.Getenv(paramMCPToolsEnabled); mcpToolsEnabled != "" {
		if mcpToolsEnabled == "true" || mcpToolsEnabled == "1" {
			config.MCPToolsEnabled = true
			logger.Debug("MCP tools enabled from environment variable", "variable", paramMCPToolsEnabled)
		} else if mcpToolsEnabled == "false" || mcpToolsEnabled == "0" {
			config.MCPToolsEnabled = false
			logger.Debug("MCP tools disabled from environment variable", "variable", paramMCPToolsEnabled)
		}
	}
	if mcpResourcesEnabled := os.Getenv(paramMCPResourcesEnabled); mcpResourcesEnabled != "" {
		if mcpResourcesEnabled == "true" || mcpResourcesEnabled == "1" {
			config.MCPResourcesEnabled = true
			logger.Debug("MCP resources enabled from environment variable", "variable", paramMCPResourcesEnabled)
		} else if mcpResourcesEnabled == "false" || mcpResourcesEnabled == "0" {
			config.MCPResourcesEnabled = false
			logger.Debug("MCP resources disabled from environment variable", "variable", paramMCPResourcesEnabled)
		}
	}
	if mcpFileToolRoot := os.Getenv(paramMCPFileToolRoot); mcpFileToolRoot != "" {
		config.MCPFileToolRoot = mcpFileToolRoot
		logger.Debug("MCP file tool root set from environment variable", "variable", paramMCPFileToolRoot, "root", mcpFileToolRoot)
	}
	if mcpDev := os.Getenv(paramMCPDev); mcpDev != "" {
		if mcpDev == "true" || mcpDev == "1" {
			config.MCPDev = true
			logger.Debug("MCP developer mode enabled from environment variable", "variable", paramMCPDev)
		} else if mcpDev == "false" || mcpDev == "0" {
			config.MCPDev = false
			logger.Debug("MCP developer mode disabled from environment variable", "variable", paramMCPDev)
		}
	}
	if mcpObservability := os.Getenv(paramMCPObservability); mcpObservability != "" {
		if mcpObservability == "true" || mcpObservability == "1" {
			config.MCPObservability = true
			logger.Debug("MCP observability enabled from environment variable", "variable", paramMCPObservability)
		} else if mcpObservability == "false" || mcpObservability == "0" {
			config.MCPObservability = false
			logger.Debug("MCP observability disabled from environment variable", "variable", paramMCPObservability)
		}
	}
	if mcpTransport := os.Getenv(paramMCPTransport); mcpTransport != "" {
		if mcpTransport == "stdio" {
			config.MCPTransport = StdioTransport
		} else if mcpTransport == "http" {
			config.MCPTransport = HTTPTransport
		}
		logger.Debug("MCP transport set from environment variable", "variable", paramMCPTransport, "transport", mcpTransport)
	}
	
	// CSP (Content Security Policy) environment variables
	if cspWebWorkerSupport := os.Getenv(paramCSPWebWorkerSupport); cspWebWorkerSupport != "" {
		if cspWebWorkerSupport == "true" || cspWebWorkerSupport == "1" {
			config.CSPWebWorkerSupport = true
			logger.Debug("CSP Web Worker support enabled from environment variable", "variable", paramCSPWebWorkerSupport)
		} else if cspWebWorkerSupport == "false" || cspWebWorkerSupport == "0" {
			config.CSPWebWorkerSupport = false
			logger.Debug("CSP Web Worker support disabled from environment variable", "variable", paramCSPWebWorkerSupport)
		}
	}
	
	// Logging environment variables
	if logLevel := os.Getenv(paramLogLevel); logLevel != "" {
		config.LogLevel = logLevel
		logger.Debug("Log level set from environment variable", "variable", paramLogLevel, "level", logLevel)
	}
	if debugMode := os.Getenv(paramDebugMode); debugMode != "" {
		if debugMode == "true" || debugMode == "1" {
			config.DebugMode = true
			config.LogLevel = "DEBUG" // Debug mode implies debug log level
			logger.Debug("Debug mode enabled from environment variable", "variable", paramDebugMode)
		} else if debugMode == "false" || debugMode == "0" {
			config.DebugMode = false
			logger.Debug("Debug mode disabled from environment variable", "variable", paramDebugMode)
		}
	}
	
	// Banner configuration
	if suppressBanner := os.Getenv(paramSuppressBanner); suppressBanner != "" {
		if suppressBanner == "true" || suppressBanner == "1" {
			config.SuppressBanner = true
			logger.Debug("Banner suppression enabled from environment variable", "variable", paramSuppressBanner)
		} else if suppressBanner == "false" || suppressBanner == "0" {
			config.SuppressBanner = false
			logger.Debug("Banner suppression disabled from environment variable", "variable", paramSuppressBanner)
		}
	}
	
	return config
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

	for i := 0; i < baseValue.NumField(); i++ {
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

// setTimeouts helper to apply only custom values or retain the server default
func (srv *Server) setTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) {
	if readTimeout != 0 {
		srv.Options.ReadTimeout = readTimeout
		srv.httpServer.ReadTimeout = readTimeout
	}
	if writeTimeout != 0 {
		srv.Options.WriteTimeout = writeTimeout
		srv.httpServer.WriteTimeout = writeTimeout
	}
	if idleTimeout != 0 {
		srv.Options.IdleTimeout = idleTimeout
		srv.httpServer.IdleTimeout = idleTimeout
	}
}
