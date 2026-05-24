package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
)

// Consolidate error responses to maintain a consistent format.
func writeErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	response := map[string]string{"error": message}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		logger.Error("Failed to write error response", "error", err)
	}
}

// templateHandler serves HTML templates with dynamic content.
func (srv *Server) templateHandler(templateName string, data any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if err := srv.templates.ExecuteTemplate(w, templateName, data); err != nil {
			slog.Error("Error rendering template", "error", err)
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
		}
	}
}

// SSEMessage represents a Server-Sent Events message with an optional event type and data payload.
// It follows the SSE format with event and data fields that can be sent to clients.
type SSEMessage struct {
	Event string `json:"event"` // Optional: Allows sending multiple event types
	Data  any    `json:"data"`  // The actual data payload
}

// NewSSEMessage creates a new SSE message with the given data and a default "message" event type.
// This is a convenience function for creating standard SSE messages.
func NewSSEMessage(data any) *SSEMessage {
	return &SSEMessage{
		Event: "message",
		Data:  data,
	}
}

// String formats the SSE message according to the Server-Sent Events
// specification. Multi-line data is emitted as one data field per line.
func (sse *SSEMessage) String() string {
	var b strings.Builder
	event := strings.NewReplacer("\r", "", "\n", "").Replace(sse.Event)
	if event != "" {
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteByte('\n')
	}
	for line := range strings.SplitSeq(sseDataString(sse.Data), "\n") {
		b.WriteString("data: ")
		b.WriteString(strings.TrimSuffix(line, "\r"))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func sseDataString(data any) string {
	switch v := data.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(encoded)
	}
}

func (srv *Server) livezHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "alive", &srv.isRunning)
}

func (srv *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "ready", &srv.isReady)
}

func (srv *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "ok", &srv.isRunning)
}

func (srv *Server) healthHandlerHelper(w http.ResponseWriter, request *http.Request, probe string,
	status *atomic.Bool) {
	if status.Load() {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(probe)); err != nil {
			logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("unhealthy")); err != nil {
			logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	}
}
