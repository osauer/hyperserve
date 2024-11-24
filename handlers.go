package hyperserve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"
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
func templateHandler(templateName string, data interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		err := templates.ExecuteTemplate(w, templateName, data)
		if err != nil {
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
			slog.Error("Error rendering template", "error", err)
		}
	}
}

// HealthCheckHandler returns a 204 status code for health check
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

type SSEMessage struct {
	Event string `json:"event"` // Optional: Allows sending multiple event types
	Data  any    `json:"data"`  // The actual data payload
}

func NewSEEventMessage(data any) *SSEMessage {
	return &SSEMessage{
		Event: "message",
		Data:  data,
	}
}

func (sse *SSEMessage) String() string {
	str := fmt.Sprintf("event: %s\ndata: %v\n\n", sse.Event, sse.Data)
	return fmt.Sprintf(str)
}

// SSEHandler is a server-sent events handler that streams data to the client.
func SSEHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(1 * time.Second) // Slower for testing
	defer ticker.Stop()

	logger.Info("SSE connection opened")
	logger.Info("event: message\ndata: Hello, world!")
	for {
		select {
		case <-r.Context().Done():
			logger.Info("SSE connection closed")
			return
		case <-ticker.C:
			number := rand.Intn(100)
			// Log the number being sent
			logger.Info("Sending number:", number)

			// Send HTMX-compatible SSE data
			fmt.Fprintf(w, "data:", number)
			flusher.Flush()
		}
	}
}

func (srv *Server) livezHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "alive", &isRunning)
}

func (srv *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "ready", &isReady)
}

func (srv *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
	srv.healthHandlerHelper(w, r, "ok", &isRunning)
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

// PanicHandler simulations a panic situation in a handler to test proper recovery. See
func PanicHandler(w http.ResponseWriter, r *http.Request) {
	panic("Intentional panic.")
}