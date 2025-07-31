package hyperserve

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

// ConfigResource implements MCPResource for server configuration access
type ConfigResource struct {
	options *ServerOptions
}

// NewConfigResource creates a new configuration resource
func NewConfigResource(options *ServerOptions) *ConfigResource {
	return &ConfigResource{options: options}
}

func (r *ConfigResource) URI() string {
	return "config://server/options"
}

func (r *ConfigResource) Name() string {
	return "Server Configuration"
}

func (r *ConfigResource) Description() string {
	return "Current server configuration settings"
}

func (r *ConfigResource) MimeType() string {
	return "application/json"
}

func (r *ConfigResource) Read() (interface{}, error) {
	// Return a sanitized version of the configuration (no sensitive data)
	config := map[string]interface{}{
		"addr":                r.options.Addr,
		"enableTLS":          r.options.EnableTLS,
		"tlsAddr":            r.options.TLSAddr,
		"healthAddr":         r.options.HealthAddr,
		"rateLimit":          float64(r.options.RateLimit),
		"burst":              r.options.Burst,
		"readTimeout":        r.options.ReadTimeout.String(),
		"writeTimeout":       r.options.WriteTimeout.String(),
		"idleTimeout":        r.options.IdleTimeout.String(),
		"staticDir":          r.options.StaticDir,
		"templateDir":        r.options.TemplateDir,
		"runHealthServer":    r.options.RunHealthServer,
		"chaosMode":          r.options.ChaosMode,
		"fipsMode":           r.options.FIPSMode,
		"hardenedMode":       r.options.HardenedMode,
		"enableECH":          r.options.EnableECH,
	}
	
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal configuration: %w", err)
	}
	
	return string(configJSON), nil
}

func (r *ConfigResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// MetricsResource implements MCPResource for server metrics access
type MetricsResource struct {
	server *Server
}

// NewMetricsResource creates a new metrics resource
func NewMetricsResource(server *Server) *MetricsResource {
	return &MetricsResource{server: server}
}

func (r *MetricsResource) URI() string {
	return "metrics://server/stats"
}

func (r *MetricsResource) Name() string {
	return "Server Metrics"
}

func (r *MetricsResource) Description() string {
	return "Current server performance metrics and statistics"
}

func (r *MetricsResource) MimeType() string {
	return "application/json"
}

func (r *MetricsResource) Read() (interface{}, error) {
	uptime := time.Since(r.server.serverStart)
	totalRequests := r.server.totalRequests.Load()
	totalResponseTime := r.server.totalResponseTime.Load()
	
	var avgResponseTime float64
	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests)
	}
	
	metrics := map[string]interface{}{
		"uptime":           uptime.String(),
		"totalRequests":    totalRequests,
		"totalResponseTime": fmt.Sprintf("%dμs", totalResponseTime),
		"avgResponseTime":  fmt.Sprintf("%.2fμs", avgResponseTime),
		"isRunning":        r.server.isRunning.Load(),
		"isReady":          r.server.isReady.Load(),
		"timestamp":        time.Now().Format(time.RFC3339),
	}
	
	metricsJSON, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics: %w", err)
	}
	
	return string(metricsJSON), nil
}

func (r *MetricsResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// SystemResource implements MCPResource for system information
type SystemResource struct{}

// NewSystemResource creates a new system resource
func NewSystemResource() *SystemResource {
	return &SystemResource{}
}

func (r *SystemResource) URI() string {
	return "system://runtime/info"
}

func (r *SystemResource) Name() string {
	return "System Information"
}

func (r *SystemResource) Description() string {
	return "Runtime system information and Go environment details"
}

func (r *SystemResource) MimeType() string {
	return "application/json"
}

func (r *SystemResource) Read() (interface{}, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	systemInfo := map[string]interface{}{
		"go": map[string]interface{}{
			"version":      runtime.Version(),
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"numCPU":       runtime.NumCPU(),
			"numGoroutine": runtime.NumGoroutine(),
		},
		"memory": map[string]interface{}{
			"allocated":     memStats.Alloc,
			"totalAlloc":    memStats.TotalAlloc,
			"sys":           memStats.Sys,
			"numGC":         memStats.NumGC,
			"gcCPUFraction": memStats.GCCPUFraction,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}
	
	systemJSON, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal system info: %w", err)
	}
	
	return string(systemJSON), nil
}

func (r *SystemResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// LogResource implements MCPResource for recent log entries (if available)
type LogResource struct {
	entries []string
	maxSize int
}

// NewLogResource creates a new log resource with a maximum number of entries
func NewLogResource(maxSize int) *LogResource {
	return &LogResource{
		entries: make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

func (r *LogResource) URI() string {
	return "logs://server/recent"
}

func (r *LogResource) Name() string {
	return "Recent Log Entries"
}

func (r *LogResource) Description() string {
	return "Recent server log entries (if log capture is enabled)"
}

func (r *LogResource) MimeType() string {
	return "text/plain"
}

func (r *LogResource) Read() (interface{}, error) {
	if len(r.entries) == 0 {
		return "No log entries captured. Log capture may not be enabled.", nil
	}
	
	result := ""
	for _, entry := range r.entries {
		result += entry + "\n"
	}
	
	return result, nil
}

func (r *LogResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// AddLogEntry adds a log entry to the resource (called by log handler if implemented)
func (r *LogResource) AddLogEntry(entry string) {
	if len(r.entries) >= r.maxSize {
		// Remove oldest entry
		r.entries = r.entries[1:]
	}
	r.entries = append(r.entries, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), entry))
}