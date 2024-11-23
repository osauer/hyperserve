package hyperserve

import (
	"encoding/json"
	"log/slog"
	"os"
	"time"
)

// ServerOptions is a representation of the Server settings
type ServerOptions struct {
	Addr              string        `json:"addr,omitempty"`
	EnableTLS         bool          `json:"tls,omitempty"`
	TLSAddr           string        `json:"tls_addr,omitempty"`
	TLSHealthAddr     string        `json:"tls_health_addr,omitempty"`
	KeyFile           string        `json:"key_file,omitempty"`
	CertFile          string        `json:"cert_file,omitempty"`
	HealthAddr        string        `json:"health_addr,omitempty"`
	RateLimit         rateLimit     `json:"rate_limit,omitempty"`
	Burst             int           `json:"burst,omitempty"`
	ReadTimeout       time.Duration `json:"read_timeout,omitempty"`
	WriteTimeout      time.Duration `json:"write_timeout,omitempty"`
	IdleTimeout       time.Duration `json:"idle_timeout,omitempty"`
	StaticDir         string        `json:"static_dir,omitempty"`
	TemplateDir       string        `json:"template_dir,omitempty"`
	RunHealthServer   bool          `json:"run_health_server,omitempty"`
	ChaosMode         bool          `json:"chaos_mode,omitempty"`
	ChaosMaxLatency   time.Duration `json:"chaos_max_latency,omitempty"`
	ChaosMinLatency   time.Duration `json:"chaos_min_latency,omitempty"`
	ChaosErrorRate    float64       `json:"chaos_error_rate,omitempty"`
	ChaosThrottleRate float64       `json:"chaos_throttle_rate,omitempty"`
	ChaosPanicRate    float64       `json:"chaos_panic_rate,omitempty"`
}

var defaultServerOptions = &ServerOptions{
	Addr:              ":8080",
	TLSAddr:           ":8443",
	HealthAddr:        ":9080",
	TLSHealthAddr:     ":9443",
	EnableTLS:         false,
	KeyFile:           "server.key",
	CertFile:          "server.crt",
	RateLimit:         1,
	Burst:             10,
	ReadTimeout:       5 * time.Second,
	WriteTimeout:      10 * time.Second,
	IdleTimeout:       120 * time.Second,
	StaticDir:         "static/",
	TemplateDir:       "template/",
	RunHealthServer:   true,
	ChaosMode:         true,
	ChaosMaxLatency:   2 * time.Second,
	ChaosMinLatency:   500 * time.Millisecond,
	ChaosErrorRate:    0.1,
	ChaosThrottleRate: 0.05,
	ChaosPanicRate:    0.01,
}

// Wrappers for debug levels to be used in the server. We're using slog for logging,
// but want to hide this detail from the client
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// NewServerOptions creates a new configuration for the server with a priority order. Environment variables override options file.
// 1. Environment variables
// 2. ServerOptions file (JSON)
// 3. Default values
func NewServerOptions() *ServerOptions {
	config := applyEnvVars(applyConfigFile(defaultServerOptions))
	return config
}

type ServerOptionFunc func(srv *Server)

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
func mergeConfig(base *ServerOptions, override *ServerOptions) {
	// todo: check all options options are covered. Can we automate this?
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