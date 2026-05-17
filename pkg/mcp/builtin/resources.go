package builtin

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

// SystemResource exposes Go runtime information (no server dependency).
type SystemResource struct{}

// NewSystemResource creates a SystemResource.
func NewSystemResource() *SystemResource { return &SystemResource{} }

func (r *SystemResource) URI() string  { return "system://runtime/info" }
func (r *SystemResource) Name() string { return "System Information" }
func (r *SystemResource) Description() string {
	return "Runtime system information and Go environment details"
}
func (r *SystemResource) MimeType() string { return "application/json" }

func (r *SystemResource) Read() (any, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	systemInfo := map[string]any{
		"go": map[string]any{
			"version":      runtime.Version(),
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"numCPU":       runtime.NumCPU(),
			"numGoroutine": runtime.NumGoroutine(),
		},
		"memory": map[string]any{
			"allocated":     memStats.Alloc,
			"totalAlloc":    memStats.TotalAlloc,
			"sys":           memStats.Sys,
			"numGC":         memStats.NumGC,
			"gcCPUFraction": memStats.GCCPUFraction,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}
	jsonBytes, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal system info: %w", err)
	}
	return string(jsonBytes), nil
}

func (r *SystemResource) List() ([]string, error) { return []string{r.URI()}, nil }
