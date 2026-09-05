package hyperserve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// Consolidate error responses to maintain a consistent format.
func writeErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	response := map[string]string{"error": message}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		slog.Default().Error("Failed to write error response", "error", err)
	}
}

// templateHandler serves HTML templates with dynamic content.
func (srv *Server) templateHandler(templateName string, data any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if err := srv.templates.ExecuteTemplate(w, templateName, data); err != nil {
			srv.logger.Error("Error rendering template", "error", err)
			http.Error(w, "Error rendering template", http.StatusInternalServerError)
		}
	}
}

// SSEMessage represents a Server-Sent Events message with a data payload,
// an optional event type, and an optional event ID.
type SSEMessage struct {
	Event string `json:"event"` // Optional: Allows sending multiple event types
	Data  any    `json:"data"`  // The actual data payload

	// ID sets the client's last event ID. Empty IDs are omitted, leaving the
	// client's previous ID unchanged; they do not emit an empty-ID reset.
	// String omits IDs containing CR, LF, NUL, or invalid UTF-8 rather than
	// changing their value. Applications own ID assignment and replay.
	ID string `json:"id,omitempty"`
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
	if sse.ID != "" && !strings.ContainsAny(sse.ID, "\r\n\x00") && utf8.ValidString(sse.ID) {
		b.WriteString("id: ")
		b.WriteString(sse.ID)
		b.WriteByte('\n')
	}
	data := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(sseDataString(sse.Data))
	for line := range strings.SplitSeq(data, "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
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
			srv.logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte("unhealthy")); err != nil {
			srv.logger.Error(fmt.Sprintf("error writing endpoint status (%s)", probe), "error", err)
		}
	}
}
