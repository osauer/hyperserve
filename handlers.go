package hyperserve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
func (srv *Server) templateHandler(templateName string, data interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if err := srv.templates.ExecuteTemplate(w, templateName, data); err != nil {
			slog.Error("Error rendering template", "error", err)
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
		}
	}
}

// HealthCheckHandler returns a 204 No Content status code for basic health checks.
// This handler can be used as a simple liveness or readiness probe.
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
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

// String formats the SSE message according to the Server-Sent Events specification.
// Returns a string in the format "event: <event>\ndata: <data>\n\n".
func (sse *SSEMessage) String() string {
	return fmt.Sprintf("event: %s\ndata: %v\n\n", sse.Event, sse.Data)
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

// PanicHandler simulates a panic situation in a handler to test proper recovery middleware.
// This handler is intended for testing purposes only and should not be used in production.
func PanicHandler(w http.ResponseWriter, r *http.Request) {
	panic("Intentional panic.")
}
