// Package ws provides low-level WebSocket protocol implementation.
// This is an internal package not meant for direct public use.
package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const (
	// WebSocket magic string defined in RFC 6455
	websocketMagicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	// WebSocket version
	websocketVersion = "13"
)

// Errors
var (
	ErrNotWebSocket      = errors.New("not a websocket handshake")
	ErrBadHandshake      = errors.New("bad handshake")
	ErrUnsupportedVersion = errors.New("unsupported websocket version")
	ErrMissingKey        = errors.New("missing Sec-WebSocket-Key")
)

// HandshakeOptions contains options for WebSocket handshake
type HandshakeOptions struct {
	// CheckOrigin returns true if the request Origin header is acceptable
	CheckOrigin func(r *http.Request) bool
	// Subprotocols is the list of supported subprotocols
	Subprotocols []string
	// Extensions is the list of supported extensions
	Extensions []string
	// BeforeUpgrade is called before the upgrade response is sent
	BeforeUpgrade func(w http.ResponseWriter, r *http.Request) error
}

// ValidateHandshake validates a WebSocket upgrade request
func ValidateHandshake(r *http.Request) error {
	// Check basic WebSocket upgrade requirements
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") ||
		r.Method != "GET" {
		return ErrNotWebSocket
	}
	
	// Check for required headers
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		return ErrMissingKey
	}
	
	if r.Header.Get("Sec-WebSocket-Version") != websocketVersion {
		return ErrUnsupportedVersion
	}
	
	return nil
}

// PerformHandshake performs the WebSocket handshake
func PerformHandshake(w http.ResponseWriter, r *http.Request, opts *HandshakeOptions) (net.Conn, *bufio.ReadWriter, error) {
	// Validate the handshake
	if err := ValidateHandshake(r); err != nil {
		return nil, nil, err
	}
	
	// Check origin if function is provided
	if opts != nil && opts.CheckOrigin != nil && !opts.CheckOrigin(r) {
		return nil, nil, ErrBadHandshake
	}
	
	// Call before upgrade hook if provided
	if opts != nil && opts.BeforeUpgrade != nil {
		if err := opts.BeforeUpgrade(w, r); err != nil {
			return nil, nil, err
		}
	}
	
	// Get the connection using hijacker
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("responsewriter does not support hijacking")
	}
	
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}
	
	// Generate accept key
	key := r.Header.Get("Sec-WebSocket-Key")
	acceptKey := generateAcceptKey(key)
	
	// Build response headers
	headers := make(http.Header)
	headers.Set("Upgrade", "websocket")
	headers.Set("Connection", "Upgrade")
	headers.Set("Sec-WebSocket-Accept", acceptKey)
	
	// Handle subprotocol negotiation
	if opts != nil && len(opts.Subprotocols) > 0 {
		clientProtocols := parseSubprotocols(r.Header.Get("Sec-WebSocket-Protocol"))
		if protocol := negotiateSubprotocol(clientProtocols, opts.Subprotocols); protocol != "" {
			headers.Set("Sec-WebSocket-Protocol", protocol)
		}
	}
	
	// Send upgrade response
	response := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n")
	for k, v := range headers {
		for _, vv := range v {
			response += fmt.Sprintf("%s: %s\r\n", k, vv)
		}
	}
	response += "\r\n"
	
	if _, err := conn.Write([]byte(response)); err != nil {
		conn.Close()
		return nil, nil, err
	}
	
	return conn, buf, nil
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		r.Method == "GET"
}

// generateAcceptKey generates the Sec-WebSocket-Accept header value
func generateAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketMagicString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// parseSubprotocols parses the Sec-WebSocket-Protocol header
func parseSubprotocols(header string) []string {
	if header == "" {
		return nil
	}
	protocols := strings.Split(header, ",")
	for i := range protocols {
		protocols[i] = strings.TrimSpace(protocols[i])
	}
	return protocols
}

// negotiateSubprotocol selects the first matching subprotocol
func negotiateSubprotocol(client, server []string) string {
	for _, c := range client {
		for _, s := range server {
			if c == s {
				return c
			}
		}
	}
	return ""
}