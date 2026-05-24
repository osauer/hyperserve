package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/osauer/hyperserve/pkg/server"
)

// ServerConfigResource exposes a sanitized snapshot of server configuration.
type ServerConfigResource struct {
	server *server.Server
}

// NewServerConfigResource creates a ServerConfigResource.
func NewServerConfigResource(srv *server.Server) *ServerConfigResource {
	return &ServerConfigResource{server: srv}
}

func (r *ServerConfigResource) URI() string  { return "config://server/current" }
func (r *ServerConfigResource) Name() string { return "Server Configuration" }
func (r *ServerConfigResource) Description() string {
	return "Current server configuration and runtime settings"
}
func (r *ServerConfigResource) MimeType() string { return "application/json" }

func (r *ServerConfigResource) Read() (any, error) {
	if r.server == nil {
		return nil, fmt.Errorf("server not initialized")
	}
	// SECURITY: sensitive fields are explicitly excluded:
	//   KeyFile/CertFile (TLS material), AuthTokenValidatorFunc, StaticDir/TemplateDir,
	//   MCPFileToolRoot. These could leak internal paths or secrets.
	opts := r.server.Options
	return map[string]any{
		"version":       server.Version,
		"build_hash":    server.BuildHash,
		"build_time":    server.BuildTime,
		"go_version":    runtime.Version(),
		"addr":          opts.Addr,
		"health_addr":   opts.HealthAddr,
		"tls_enabled":   opts.EnableTLS,
		"rate_limit":    opts.RateLimit,
		"burst":         opts.Burst,
		"hardened_mode": opts.HardenedMode,
		"fips_mode":     opts.FIPSMode,
		"mcp_enabled":   opts.MCPEnabled,
		"mcp_endpoint":  opts.MCPEndpoint,
		"debug_mode":    opts.DebugMode,
		"log_level":     opts.LogLevel,
		"timeouts": map[string]string{
			"read":  opts.ReadTimeout.String(),
			"write": opts.WriteTimeout.String(),
			"idle":  opts.IdleTimeout.String(),
		},
		"middleware_count": len(r.server.MiddlewareRoutes()),
		"is_running":       r.server.IsRunning(),
		"is_ready":         r.server.IsReady(),
	}, nil
}

func (r *ServerConfigResource) List() ([]string, error) { return []string{r.URI()}, nil }

// ServerHealthResource exposes a snapshot of server health.
type ServerHealthResource struct {
	server *server.Server
}

// NewServerHealthResource creates a ServerHealthResource.
func NewServerHealthResource(srv *server.Server) *ServerHealthResource {
	return &ServerHealthResource{server: srv}
}

func (r *ServerHealthResource) URI() string  { return "health://server/status" }
func (r *ServerHealthResource) Name() string { return "Server Health Status" }
func (r *ServerHealthResource) Description() string {
	return "Current server health, readiness, and liveness status"
}
func (r *ServerHealthResource) MimeType() string { return "application/json" }

func (r *ServerHealthResource) Read() (any, error) {
	if r.server == nil {
		return nil, fmt.Errorf("server not initialized")
	}
	start := r.server.ServerStart()
	uptime := time.Duration(0)
	if !start.IsZero() {
		uptime = time.Since(start)
	}
	return map[string]any{
		"status": map[string]bool{
			"alive":   r.server.IsRunning(),
			"ready":   r.server.IsReady(),
			"healthy": r.server.IsRunning() && r.server.IsReady(),
		},
		"uptime":         uptime.String(),
		"uptime_seconds": int(uptime.Seconds()),
		"metrics": map[string]any{
			"total_requests":       r.server.TotalRequests(),
			"total_response_time":  r.server.TotalResponseTime(),
			"avg_response_time_us": avgResponseTime(r.server),
			"active_limiters":      r.server.ClientLimiterCount(),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}, nil
}

func (r *ServerHealthResource) List() ([]string, error) { return []string{r.URI()}, nil }

// ServerLogResource buffers recent log entries for retrieval via MCP.
// It implements slog.Handler so it can be slotted into the logging chain.
type ServerLogResource struct {
	mu      sync.RWMutex
	logs    []logEntry
	maxSize int
	handler slog.Handler
}

type logEntry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"msg"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// NewServerLogResource creates a ServerLogResource with the given buffer size.
// A non-positive size defaults to 100.
func NewServerLogResource(maxSize int) *ServerLogResource {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &ServerLogResource{
		logs:    make([]logEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

func (r *ServerLogResource) URI() string  { return "logs://server/recent" }
func (r *ServerLogResource) Name() string { return "Server Logs" }
func (r *ServerLogResource) Description() string {
	return fmt.Sprintf("Recent server logs (last %d entries)", r.maxSize)
}
func (r *ServerLogResource) MimeType() string { return "application/json" }

func (r *ServerLogResource) Read() (any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	logsCopy := make([]logEntry, len(r.logs))
	copy(logsCopy, r.logs)

	logData := map[string]any{
		"logs":      logsCopy,
		"count":     len(logsCopy),
		"max_size":  r.maxSize,
		"truncated": len(r.logs) >= r.maxSize,
	}
	jsonBytes, err := json.Marshal(logData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal log data: %w", err)
	}
	return string(jsonBytes), nil
}

func (r *ServerLogResource) List() ([]string, error) { return []string{r.URI()}, nil }

// Handle implements slog.Handler so the resource can capture log records.
func (r *ServerLogResource) Handle(ctx context.Context, record slog.Record) error {
	entry := logEntry{
		Time:    record.Time,
		Level:   record.Level.String(),
		Message: record.Message,
		Attrs:   make(map[string]any),
	}
	record.Attrs(func(attr slog.Attr) bool {
		entry.Attrs[attr.Key] = attr.Value.Any()
		return true
	})

	r.mu.Lock()
	if len(r.logs) >= r.maxSize {
		r.logs = r.logs[1:]
	}
	r.logs = append(r.logs, entry)
	handler := r.handler
	r.mu.Unlock()

	if handler != nil {
		return handler.Handle(ctx, record)
	}
	return nil
}

func (r *ServerLogResource) Enabled(ctx context.Context, level slog.Level) bool {
	if r.handler != nil {
		return r.handler.Enabled(ctx, level)
	}
	return true
}

func (r *ServerLogResource) WithAttrs(attrs []slog.Attr) slog.Handler { return r }
func (r *ServerLogResource) WithGroup(name string) slog.Handler       { return r }

// StreamingLogResource is a ServerLogResource exposed at a different URI to
// distinguish "stream" semantics (intended for the dev workflow) from the
// generic "recent" view.
type StreamingLogResource struct {
	*ServerLogResource
}

func (r *StreamingLogResource) URI() string  { return "logs://server/stream" }
func (r *StreamingLogResource) Name() string { return "Server Log Stream" }
func (r *StreamingLogResource) Description() string {
	return "Real-time server log streaming for development"
}

// RouteListResource provides a structured list of registered routes.
type RouteListResource struct {
	server *server.Server
}

// NewRouteListResource creates a RouteListResource.
func NewRouteListResource(srv *server.Server) *RouteListResource {
	return &RouteListResource{server: srv}
}

func (r *RouteListResource) URI() string         { return "routes://server/all" }
func (r *RouteListResource) Name() string        { return "Server Routes" }
func (r *RouteListResource) Description() string { return "All registered routes with metadata" }
func (r *RouteListResource) MimeType() string    { return "application/json" }

func (r *RouteListResource) Read() (any, error) {
	routes := []map[string]any{}
	for _, route := range r.server.RegisteredRoutes() {
		pattern, methods := splitServeMuxPattern(route)
		routes = append(routes, map[string]any{
			"pattern": pattern,
			"methods": methods,
		})
	}
	return map[string]any{
		"routes": routes,
		"count":  len(routes),
	}, nil
}

func (r *RouteListResource) List() ([]string, error) { return []string{r.URI()}, nil }

// ConfigResource wraps a *server.ServerOptions for read-only exposure.
type ConfigResource struct {
	options *server.ServerOptions
}

// NewConfigResource creates a ConfigResource.
func NewConfigResource(options *server.ServerOptions) *ConfigResource {
	return &ConfigResource{options: options}
}

func (r *ConfigResource) URI() string         { return "config://server/options" }
func (r *ConfigResource) Name() string        { return "Server Configuration" }
func (r *ConfigResource) Description() string { return "Current server configuration settings" }
func (r *ConfigResource) MimeType() string    { return "application/json" }

func (r *ConfigResource) Read() (any, error) {
	config := map[string]any{
		"addr":            r.options.Addr,
		"enableTLS":       r.options.EnableTLS,
		"tlsAddr":         r.options.TLSAddr,
		"healthAddr":      r.options.HealthAddr,
		"rateLimit":       float64(r.options.RateLimit),
		"burst":           r.options.Burst,
		"readTimeout":     r.options.ReadTimeout.String(),
		"writeTimeout":    r.options.WriteTimeout.String(),
		"idleTimeout":     r.options.IdleTimeout.String(),
		"staticDir":       r.options.StaticDir,
		"templateDir":     r.options.TemplateDir,
		"runHealthServer": r.options.RunHealthServer,
		"fipsMode":        r.options.FIPSMode,
		"hardenedMode":    r.options.HardenedMode,
	}
	jsonBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal configuration: %w", err)
	}
	return string(jsonBytes), nil
}

func (r *ConfigResource) List() ([]string, error) { return []string{r.URI()}, nil }

// MetricsResource exposes server metrics as JSON.
type MetricsResource struct {
	server *server.Server
}

// NewMetricsResource creates a MetricsResource.
func NewMetricsResource(srv *server.Server) *MetricsResource {
	return &MetricsResource{server: srv}
}

func (r *MetricsResource) URI() string  { return "metrics://server/stats" }
func (r *MetricsResource) Name() string { return "Server Metrics" }
func (r *MetricsResource) Description() string {
	return "Current server performance metrics and statistics"
}
func (r *MetricsResource) MimeType() string { return "application/json" }

func (r *MetricsResource) Read() (any, error) {
	uptime := time.Since(r.server.ServerStart())
	totalRequests := r.server.TotalRequests()
	totalResponseTime := r.server.TotalResponseTime()

	var avgResp float64
	if totalRequests > 0 {
		avgResp = float64(totalResponseTime) / float64(totalRequests)
	}
	metrics := map[string]any{
		"uptime":            uptime.String(),
		"totalRequests":     totalRequests,
		"totalResponseTime": fmt.Sprintf("%dμs", totalResponseTime),
		"avgResponseTime":   fmt.Sprintf("%.2fμs", avgResp),
		"isRunning":         r.server.IsRunning(),
		"isReady":           r.server.IsReady(),
		"timestamp":         time.Now().Format(time.RFC3339),
	}
	jsonBytes, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics: %w", err)
	}
	return string(jsonBytes), nil
}

func (r *MetricsResource) List() ([]string, error) { return []string{r.URI()}, nil }

// avgResponseTime computes the average response time in microseconds. Returns
// 0 if no requests have been processed yet (or in the unlikely overflow case).
func avgResponseTime(srv *server.Server) int64 {
	requests := srv.TotalRequests()
	if requests == 0 {
		return 0
	}
	if requests > math.MaxInt64 {
		return 0
	}
	return srv.TotalResponseTime() / int64(requests) //nolint:gosec // checked above
}
