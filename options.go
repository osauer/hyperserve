package hyperserve

import (
	"encoding/json"
	"log/slog"
	"os"
	"reflect"
	"time"
)

// ServerOptions contains all configuration settings for the HTTP server.
// Options are loaded from environment variables, configuration files, and defaults in that priority order.
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
	mcpTransportOpts       mcpTransportOptions // Internal transport options
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
	ReadTimeout:            5 * time.Second,
	WriteTimeout:           10 * time.Second,
	IdleTimeout:            120 * time.Second,
	StaticDir:              "static/",
	TemplateDir:            "template/",
	RunHealthServer:        true,
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
	MCPToolsEnabled:        true,
	MCPResourcesEnabled:    true,
	MCPFileToolRoot:        "",
	MCPLogResourceSize:     100,
	MCPTransport:           HTTPTransport,
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
		logger.Info("Server address set from environment variable", "variable", paramServerAddr, "addr", addr)
	}
	if healthAddr := os.Getenv(paramHealthAddr); healthAddr != "" {
		config.HealthAddr = healthAddr
		logger.Info("Health endpoint address set from environment variable", "variable", paramHealthAddr, "addr", healthAddr)
	}
	if hardenedMode := os.Getenv(paramHardenedMode); hardenedMode != "" {
		if hardenedMode == "true" || hardenedMode == "1" {
			config.HardenedMode = true
			logger.Info("Hardened mode enabled from environment variable", "variable", paramHardenedMode)
		}
	}
	
	// MCP (Model Context Protocol) environment variables
	if mcpEnabled := os.Getenv(paramMCPEnabled); mcpEnabled != "" {
		if mcpEnabled == "true" || mcpEnabled == "1" {
			config.MCPEnabled = true
			logger.Info("MCP enabled from environment variable", "variable", paramMCPEnabled)
		} else if mcpEnabled == "false" || mcpEnabled == "0" {
			config.MCPEnabled = false
			logger.Info("MCP disabled from environment variable", "variable", paramMCPEnabled)
		}
	}
	if mcpEndpoint := os.Getenv(paramMCPEndpoint); mcpEndpoint != "" {
		config.MCPEndpoint = mcpEndpoint
		logger.Info("MCP endpoint set from environment variable", "variable", paramMCPEndpoint, "endpoint", mcpEndpoint)
	}
	if mcpServerName := os.Getenv(paramMCPServerName); mcpServerName != "" {
		config.MCPServerName = mcpServerName
		logger.Info("MCP server name set from environment variable", "variable", paramMCPServerName, "name", mcpServerName)
	}
	if mcpServerVersion := os.Getenv(paramMCPServerVersion); mcpServerVersion != "" {
		config.MCPServerVersion = mcpServerVersion
		logger.Info("MCP server version set from environment variable", "variable", paramMCPServerVersion, "version", mcpServerVersion)
	}
	if mcpFileToolRoot := os.Getenv(paramMCPFileToolRoot); mcpFileToolRoot != "" {
		config.MCPFileToolRoot = mcpFileToolRoot
		logger.Info("MCP file tool root set from environment variable", "variable", paramMCPFileToolRoot, "root", mcpFileToolRoot)
	}
	
	return config
}

// helper to read a options file and apply it to the options
func applyConfigFile(config *ServerOptions) *ServerOptions {
	file, err := os.Open(paramFileName)
	if err != nil {
		logger.Warn("Failed to open options file.", "error", err)
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
		logger.Info("No options file or loading failed; Using environment and defaults")
		return config
	}
	logger.Info("Server configuration loaded from file", "file", paramFileName)
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
