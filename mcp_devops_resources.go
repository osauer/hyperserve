// Package hyperserve provides MCP DevOps resources for server monitoring.
//
// Security Considerations:
//   - No sensitive data is exposed through these resources
//   - TLS keys, certificates, and authentication functions are excluded
//   - File paths are sanitized to prevent directory traversal information leakage
//   - Only runtime metrics and sanitized configuration are exposed
//   - Logs are limited to a fixed buffer size to prevent memory exhaustion
package hyperserve

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// DevOps MCP Resources for server introspection and monitoring

// ServerConfigResource provides access to the current server configuration
type ServerConfigResource struct {
	server *Server
}

// NewServerConfigResource creates a new server configuration resource
func NewServerConfigResource(srv *Server) *ServerConfigResource {
	return &ServerConfigResource{server: srv}
}

func (r *ServerConfigResource) URI() string {
	return "config://server/current"
}

func (r *ServerConfigResource) Name() string {
	return "Server Configuration"
}

func (r *ServerConfigResource) Description() string {
	return "Current server configuration and runtime settings"
}

func (r *ServerConfigResource) MimeType() string {
	return "application/json"
}

func (r *ServerConfigResource) Read() (interface{}, error) {
	if r.server == nil {
		return nil, fmt.Errorf("server not initialized")
	}

	// Create a sanitized copy of options
	// SECURITY: We explicitly exclude sensitive fields:
	// - KeyFile/CertFile: TLS private key and certificate paths
	// - AuthTokenValidatorFunc: Contains authentication logic
	// - ECHKeys: Encrypted Client Hello keys
	// - StaticDir/TemplateDir: Could expose internal file structure
	// - MCPFileToolRoot: Could expose sandboxed directory paths
	config := map[string]interface{}{
		"version":           Version,
		"build_hash":        BuildHash,
		"build_time":        BuildTime,
		"go_version":        runtime.Version(),
		"addr":              r.server.Options.Addr,
		"health_addr":       r.server.Options.HealthAddr,
		"tls_enabled":       r.server.Options.EnableTLS,
		"rate_limit":        r.server.Options.RateLimit,
		"burst":             r.server.Options.Burst,
		"hardened_mode":     r.server.Options.HardenedMode,
		"fips_mode":         r.server.Options.FIPSMode,
		"mcp_enabled":       r.server.Options.MCPEnabled,
		"mcp_endpoint":      r.server.Options.MCPEndpoint,
		"debug_mode":        r.server.Options.DebugMode,
		"log_level":         r.server.Options.LogLevel,
		"timeouts": map[string]string{
			"read":  r.server.Options.ReadTimeout.String(),
			"write": r.server.Options.WriteTimeout.String(),
			"idle":  r.server.Options.IdleTimeout.String(),
		},
		"middleware_count": len(r.server.middleware.middleware),
		"is_running":       r.server.isRunning.Load(),
		"is_ready":         r.server.isReady.Load(),
	}

	return config, nil
}

func (r *ServerConfigResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// ServerHealthResource provides access to server health status
type ServerHealthResource struct {
	server *Server
}

// NewServerHealthResource creates a new server health resource
func NewServerHealthResource(srv *Server) *ServerHealthResource {
	return &ServerHealthResource{server: srv}
}

func (r *ServerHealthResource) URI() string {
	return "health://server/status"
}

func (r *ServerHealthResource) Name() string {
	return "Server Health Status"
}

func (r *ServerHealthResource) Description() string {
	return "Current server health, readiness, and liveness status"
}

func (r *ServerHealthResource) MimeType() string {
	return "application/json"
}

func (r *ServerHealthResource) Read() (interface{}, error) {
	if r.server == nil {
		return nil, fmt.Errorf("server not initialized")
	}

	uptime := time.Duration(0)
	if !r.server.serverStart.IsZero() {
		uptime = time.Since(r.server.serverStart)
	}

	health := map[string]interface{}{
		"status": map[string]bool{
			"alive":   r.server.isRunning.Load(),
			"ready":   r.server.isReady.Load(),
			"healthy": r.server.isRunning.Load() && r.server.isReady.Load(),
		},
		"uptime":         uptime.String(),
		"uptime_seconds": int(uptime.Seconds()),
		"metrics": map[string]interface{}{
			"total_requests":       r.server.totalRequests.Load(),
			"total_response_time":  r.server.totalResponseTime.Load(),
			"avg_response_time_us": calculateAvgResponseTime(r.server),
			"active_limiters":      len(r.server.clientLimiters),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	return health, nil
}

func (r *ServerHealthResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// ServerLogResource provides access to recent server logs
type ServerLogResource struct {
	mu       sync.RWMutex
	logs     []logEntry
	maxSize  int
	handler  slog.Handler
}

type logEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"msg"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
}

// NewServerLogResource creates a new server log resource
func NewServerLogResource(maxSize int) *ServerLogResource {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &ServerLogResource{
		logs:    make([]logEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

func (r *ServerLogResource) URI() string {
	return "logs://server/recent"
}

func (r *ServerLogResource) Name() string {
	return "Server Logs"
}

func (r *ServerLogResource) Description() string {
	return fmt.Sprintf("Recent server logs (last %d entries)", r.maxSize)
}

func (r *ServerLogResource) MimeType() string {
	return "application/json"
}

func (r *ServerLogResource) Read() (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy of the logs
	logsCopy := make([]logEntry, len(r.logs))
	copy(logsCopy, r.logs)

	return map[string]interface{}{
		"logs":       logsCopy,
		"count":      len(logsCopy),
		"max_size":   r.maxSize,
		"truncated":  len(r.logs) >= r.maxSize,
	}, nil
}

func (r *ServerLogResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// Handle implements slog.Handler to capture logs
func (r *ServerLogResource) Handle(ctx context.Context, record slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := logEntry{
		Time:    record.Time,
		Level:   record.Level.String(),
		Message: record.Message,
		Attrs:   make(map[string]interface{}),
	}

	// Collect attributes
	record.Attrs(func(attr slog.Attr) bool {
		entry.Attrs[attr.Key] = attr.Value.Any()
		return true
	})

	// Add to circular buffer
	if len(r.logs) >= r.maxSize {
		// Remove oldest entry
		r.logs = r.logs[1:]
	}
	r.logs = append(r.logs, entry)

	// Forward to original handler if set
	if r.handler != nil {
		return r.handler.Handle(ctx, record)
	}
	return nil
}

func (r *ServerLogResource) Enabled(ctx context.Context, level slog.Level) bool {
	if r.handler != nil {
		return r.handler.Enabled(ctx, level)
	}
	return true
}

func (r *ServerLogResource) WithAttrs(attrs []slog.Attr) slog.Handler {
	return r
}

func (r *ServerLogResource) WithGroup(name string) slog.Handler {
	return r
}

// Helper function to calculate average response time
func calculateAvgResponseTime(srv *Server) int64 {
	requests := srv.totalRequests.Load()
	if requests == 0 {
		return 0
	}
	return srv.totalResponseTime.Load() / int64(requests)
}

// RegisterObservabilityMCPResources registers minimal observability resources for production monitoring
func (srv *Server) RegisterObservabilityMCPResources() {
	if srv.mcpHandler == nil {
		logger.Warn("Cannot register observability MCP resources: MCP handler not initialized")
		return
	}

	// Register server configuration resource
	srv.mcpHandler.RegisterResource(NewServerConfigResource(srv))
	
	// Register server health resource
	srv.mcpHandler.RegisterResource(NewServerHealthResource(srv))
	
	// Create and register log resource with custom logger
	logResource := NewServerLogResource(srv.Options.MCPLogResourceSize)
	srv.mcpHandler.RegisterResource(logResource)
	
	// If in debug mode, also intercept logs
	if srv.Options.DebugMode {
		// Create a multi-handler that writes to both original and log resource
		originalHandler := logger.Handler()
		logResource.handler = originalHandler
		multiLogger := slog.New(logResource)
		slog.SetDefault(multiLogger)
		logger = multiLogger
	}
	
	logger.Info("Observability MCP resources registered", 
		"resources", []string{"config://server/current", "health://server/status", "logs://server/recent"})
}

// MCPObservability configures MCP with observability resources for production use.
// This configuration provides read-only access to system state without any dangerous operations:
// - config://server/current - Server configuration (sanitized, no secrets)
// - health://server/status - Health metrics and uptime
// - logs://server/recent - Recent log entries (circular buffer)
//
// Safe for production use - provides observability without control plane access.
//
// Example:
//   srv, _ := hyperserve.NewServer(
//       hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPObservability()),
//   )
func MCPObservability() MCPTransportConfig {
	return func(opts *mcpTransportOptions) {
		opts.observabilityMode = true
	}
}