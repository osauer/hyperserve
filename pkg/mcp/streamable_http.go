package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

const (
	streamableHTTPMaxBody = 4 << 20

	headerProtocolVersion = "MCP-Protocol-Version"
	headerMethod          = "Mcp-Method"
	headerName            = "Mcp-Name"

	errorCodeHeaderMismatch             = -32020
	errorCodeUnsupportedProtocolVersion = -32022
)

const protocolVersionMetaKey = "io.modelcontextprotocol/protocolVersion"

// isStreamableHTTPRequest distinguishes the stateless 2026 transport from
// initialize-era requests sharing the same endpoint. A partially-formed set
// still selects the modern path so missing required headers fail closed rather
// than silently downgrading to the legacy protocol.
func (h *Handler) isStreamableHTTPRequest(r *http.Request) bool {
	// Mcp-Method, Mcp-Name, and Mcp-Param-* were introduced with the current
	// per-request metadata era. Their presence must never fall back.
	if r.Header.Get(headerMethod) != "" || r.Header.Get(headerName) != "" {
		return true
	}
	for name := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "mcp-param-") {
			return true
		}
	}

	versions := r.Header.Values(headerProtocolVersion)
	if len(versions) == 0 {
		// Absence is intentionally the initialize-era compatibility signal.
		return false
	}
	if len(versions) != 1 {
		return true
	}
	// The configured initialize-era version uses the legacy request/response
	// path. Any other value enters modern validation and either negotiates the
	// current revision or returns UnsupportedProtocolVersion.
	return versions[0] != h.protocolVersion
}

func (h *Handler) isOriginAllowed(r *http.Request) bool {
	if h.originValidator != nil {
		return h.originValidator(r)
	}

	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	origin := strings.TrimSpace(values[0])
	if origin == "" || origin == "null" || strings.Contains(origin, ",") {
		return false
	}

	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	requestScheme := "http"
	if r.TLS != nil {
		requestScheme = "https"
	}
	if !strings.EqualFold(u.Scheme, requestScheme) {
		return false
	}

	originHost, originPort := normalizedHostPort(u.Host, u.Scheme)
	requestHost, requestPort := normalizedHostPort(r.Host, requestScheme)
	return originHost != "" && strings.EqualFold(originHost, requestHost) && originPort == requestPort
}

func normalizedHostPort(hostport, scheme string) (string, string) {
	u, err := url.Parse("//" + hostport)
	if err != nil || u.Hostname() == "" {
		return "", ""
	}
	port := u.Port()
	if port == "" {
		switch strings.ToLower(scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return u.Hostname(), port
}

func acceptsExactMediaType(header, want string) bool {
	for part := range strings.SplitSeq(header, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil || !strings.EqualFold(mediaType, want) || mediaTypeQualityRejected(params["q"]) {
			continue
		}
		return true
	}
	return false
}

func mediaTypeQualityRejected(value string) bool {
	if value == "" {
		return false
	}
	quality, err := strconv.ParseFloat(value, 64)
	return err != nil || quality <= 0 || quality > 1
}

func (h *Handler) serveStreamableHTTP(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if !acceptsExactMediaType(accept, "application/json") || !acceptsExactMediaType(accept, "text/event-stream") {
		h.writeStreamableError(w, http.StatusNotAcceptable, nil, jsonrpc.ErrorCodeInvalidRequest,
			"Accept must list application/json and text/event-stream", nil)
		return
	}

	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeInvalidRequest,
			"Content-Type must be application/json", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, streamableHTTPMaxBody)
	var request jsonrpc.Request
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeParseError, "Parse error", nil)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		h.writeStreamableError(w, http.StatusBadRequest, request.ID, jsonrpc.ErrorCodeParseError,
			"Request body must contain one JSON-RPC message", nil)
		return
	}

	if status, rpcErr := h.validateStreamableRequest(r, &request); rpcErr != nil {
		h.writeStreamableError(w, status, request.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
		return
	}
	if !isCurrentProtocolMethod(request.Method) {
		h.writeStreamableError(w, http.StatusNotFound, request.ID, jsonrpc.ErrorCodeMethodNotFound,
			"Method not found", map[string]any{"method": request.Method})
		return
	}

	// A per-request session propagates disconnect cancellation into resource
	// reads and context-aware tools without creating protocol-level state.
	session := newMCPSession(r.Context(), h, nil)
	defer session.close()
	var response *jsonrpc.Response
	if request.Method == "server/discover" {
		if request.IsNotification() {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result, err := h.handleServerDiscover(request.Params)
		if err != nil {
			h.writeStreamableError(w, http.StatusInternalServerError, request.ID, jsonrpc.ErrorCodeInternalError,
				"Internal error", nil)
			return
		}
		response = &jsonrpc.Response{JSONRPC: jsonrpc.Version, Result: result, ID: request.ID}
	} else {
		response = h.newRPCEngine(session).ProcessRequestDirect(&request)
	}
	if response == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	response.Result, err = h.streamableResult(response.Result)
	if err != nil {
		h.writeStreamableError(w, http.StatusInternalServerError, request.ID, jsonrpc.ErrorCodeInternalError,
			"Internal error", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode Streamable HTTP response", "error", err)
	}
}

func (h *Handler) validateStreamableRequest(r *http.Request, request *jsonrpc.Request) (int, *jsonrpc.ErrorDetails) {
	requestedVersion, ok := singleHeader(r.Header, headerProtocolVersion)
	if !ok || requestedVersion == "" {
		return headerMismatch("MCP-Protocol-Version is required")
	}
	if requestedVersion != StreamableHTTPProtocolVersion {
		return http.StatusBadRequest, &jsonrpc.ErrorDetails{
			Code:    errorCodeUnsupportedProtocolVersion,
			Message: "Unsupported protocol version",
			Data: map[string]any{
				"requested": requestedVersion,
				"supported": []string{StreamableHTTPProtocolVersion},
			},
		}
	}

	params, _ := request.Params.(map[string]any)
	meta, _ := params["_meta"].(map[string]any)
	if bodyVersion, _ := meta[protocolVersionMetaKey].(string); bodyVersion != requestedVersion {
		return headerMismatch("MCP-Protocol-Version does not match params._meta protocolVersion")
	}
	method, ok := singleHeader(r.Header, headerMethod)
	if !ok || method == "" || method != request.Method {
		return headerMismatch("Mcp-Method is missing or does not match the request method")
	}

	wantName, required := streamableRequestName(request.Method, params)
	if required {
		nameValue, single := singleHeader(r.Header, headerName)
		gotName, err := decodeMCPHeaderValue(nameValue)
		if !single || err != nil || gotName == "" || gotName != wantName {
			return headerMismatch("Mcp-Name is missing, malformed, or does not match the request body")
		}
	} else if len(r.Header.Values(headerName)) != 0 {
		return headerMismatch("Mcp-Name is not valid for this request method")
	}
	if message := h.validateToolParameterHeaders(r.Header, request.Method, params); message != "" {
		return headerMismatch(message)
	}

	return 0, nil
}

func singleHeader(header http.Header, name string) (string, bool) {
	values := header.Values(name)
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func headerMismatch(message string) (int, *jsonrpc.ErrorDetails) {
	return http.StatusBadRequest, &jsonrpc.ErrorDetails{Code: errorCodeHeaderMismatch, Message: message}
}

func streamableRequestName(method string, params map[string]any) (string, bool) {
	var field string
	switch method {
	case "tools/call", "prompts/get":
		field = "name"
	case "resources/read":
		field = "uri"
	default:
		return "", false
	}
	value, _ := params[field].(string)
	return value, true
}

func decodeMCPHeaderValue(value string) (string, error) {
	if strings.HasPrefix(value, "=?base64?") && strings.HasSuffix(value, "?=") {
		encoded := strings.TrimSuffix(strings.TrimPrefix(value, "=?base64?"), "?=")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || !utf8.Valid(decoded) {
			return "", fmt.Errorf("invalid base64 header value")
		}
		return string(decoded), nil
	}
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("invalid plain header value")
	}
	for _, b := range []byte(value) {
		if b < 0x20 || b > 0x7e {
			return "", fmt.Errorf("invalid plain header value")
		}
	}
	return value, nil
}

type toolHeaderBinding struct {
	headerName string
	path       []string
	valueType  string
}

func (h *Handler) validateToolParameterHeaders(header http.Header, method string, params map[string]any) string {
	if method != "tools/call" {
		return ""
	}
	toolName, _ := params["name"].(string)
	tool, ok := h.tools[toolName]
	if !ok {
		return ""
	}
	bindings := collectToolHeaderBindings(tool.Schema(), nil, nil)
	arguments, _ := params["arguments"].(map[string]any)
	for _, binding := range bindings {
		value, present := nestedValue(arguments, binding.path)
		values := header.Values("Mcp-Param-" + binding.headerName)
		if !present || value == nil {
			if len(values) != 0 {
				return "Mcp-Param-" + binding.headerName + " is present without a corresponding tool argument"
			}
			continue
		}
		if len(values) != 1 {
			return "Mcp-Param-" + binding.headerName + " is missing or duplicated"
		}
		decoded, err := decodeMCPHeaderValue(values[0])
		if err != nil || !toolHeaderValueMatches(decoded, value, binding.valueType) {
			return "Mcp-Param-" + binding.headerName + " does not match the tool argument"
		}
	}
	return ""
}

func collectToolHeaderBindings(schema map[string]any, path []string, seen map[string]struct{}) []toolHeaderBinding {
	if seen == nil {
		seen = make(map[string]struct{})
	}
	properties, _ := schema["properties"].(map[string]any)
	bindings := make([]toolHeaderBinding, 0)
	for propertyName, raw := range properties {
		property, _ := raw.(map[string]any)
		propertyPath := append(append([]string(nil), path...), propertyName)
		if annotation, ok := property["x-mcp-header"].(string); ok && validHTTPToken(annotation) {
			valueType, _ := property["type"].(string)
			key := strings.ToLower(annotation)
			_, duplicate := seen[key]
			if !duplicate && (valueType == "string" || valueType == "integer" || valueType == "boolean") {
				seen[key] = struct{}{}
				bindings = append(bindings, toolHeaderBinding{
					headerName: annotation,
					path:       propertyPath,
					valueType:  valueType,
				})
			}
		}
		bindings = append(bindings, collectToolHeaderBindings(property, propertyPath, seen)...)
	}
	return bindings
}

func validHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, b := range []byte(value) {
		if ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9') {
			continue
		}
		switch b {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func nestedValue(arguments map[string]any, path []string) (any, bool) {
	var current any = arguments
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func toolHeaderValueMatches(headerValue string, bodyValue any, valueType string) bool {
	switch valueType {
	case "string":
		value, ok := bodyValue.(string)
		return ok && headerValue == value
	case "boolean":
		value, ok := bodyValue.(bool)
		return ok && headerValue == strconv.FormatBool(value)
	case "integer":
		value, ok := bodyValue.(float64)
		if !ok || math.Trunc(value) != value || math.Abs(value) > 1<<53-1 {
			return false
		}
		headerNumber, err := strconv.ParseFloat(headerValue, 64)
		return err == nil && headerNumber == value
	default:
		return false
	}
}

func isCurrentProtocolMethod(method string) bool {
	switch method {
	case "server/discover", "resources/list", "resources/templates/list", "resources/read", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}

func (h *Handler) handleServerDiscover(_ any) (any, error) {
	return map[string]any{
		"resultType":        "complete",
		"supportedVersions": []string{StreamableHTTPProtocolVersion},
		"capabilities": map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
		},
		// Registrations and authorization context can vary between deployments.
		// Do not invite shared caches to retain a capability view by default.
		"ttlMs":      0,
		"cacheScope": "private",
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": h.serverInfo,
		},
	}, nil
}

func (h *Handler) streamableResult(result any) (any, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if string(data) == "null" {
		normalized = make(map[string]any)
	} else if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}
	normalized["resultType"] = "complete"
	meta, _ := normalized["_meta"].(map[string]any)
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["io.modelcontextprotocol/serverInfo"] = h.serverInfo
	normalized["_meta"] = meta
	return normalized, nil
}

func (h *Handler) writeStreamableError(w http.ResponseWriter, status int, id any, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(&jsonrpc.Response{
		JSONRPC: jsonrpc.Version,
		Error: &jsonrpc.ErrorDetails{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}); err != nil {
		h.logger.Error("Failed to encode Streamable HTTP error", "error", err)
	}
}
