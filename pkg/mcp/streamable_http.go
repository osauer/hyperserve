package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	if versions[0] == StreamableHTTPProtocolVersion {
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

func mediaTypeQualityRejected(value string) bool {
	if value == "" {
		return false
	}
	quality, err := strconv.ParseFloat(value, 64)
	return err != nil || quality <= 0 || quality > 1
}

func hasLegacyRoutedSSEHeaders(header http.Header) bool {
	return len(header.Values("X-SSE-Client-ID")) != 0 || len(header.Values("X-SSE-Binding")) != 0
}

func validateStreamableAccept(header http.Header) bool {
	values := header.Values("Accept")
	if len(values) == 0 {
		return false
	}
	seenJSON := false
	seenSSE := false
	for part := range strings.SplitSeq(strings.Join(values, ","), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil || mediaTypeQualityRejected(params["q"]) {
			return false
		}
		switch {
		case strings.EqualFold(mediaType, "application/json"):
			seenJSON = true
		case strings.EqualFold(mediaType, "text/event-stream"):
			seenSSE = true
		}
	}
	return seenJSON && seenSSE
}

func (h *Handler) serveStreamableHTTP(w http.ResponseWriter, r *http.Request) {
	if hasLegacyRoutedSSEHeaders(r.Header) {
		h.writeStreamableError(w, http.StatusBadRequest, nil, errorCodeHeaderMismatch,
			"X-SSE-* headers are not valid for Streamable HTTP", nil)
		return
	}
	if !validateStreamableAccept(r.Header) {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeInvalidRequest,
			"Accept must list application/json and text/event-stream", nil)
		return
	}

	contentTypes := r.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		h.writeStreamableError(w, http.StatusUnsupportedMediaType, nil, jsonrpc.ErrorCodeInvalidRequest,
			"Content-Type must be a single application/json value", nil)
		return
	}
	contentType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		h.writeStreamableError(w, http.StatusUnsupportedMediaType, nil, jsonrpc.ErrorCodeInvalidRequest,
			"Content-Type must be application/json", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, streamableHTTPMaxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			h.writeStreamableError(w, http.StatusRequestEntityTooLarge, nil, jsonrpc.ErrorCodeInvalidRequest,
				"Request body is too large", nil)
			return
		}
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeParseError, "Parse error", nil)
		return
	}
	if err := validateUniqueJSON(body); err != nil {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeParseError, "Parse error", nil)
		return
	}

	var request jsonrpc.Request
	if err := json.Unmarshal(body, &request); err != nil {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeParseError, "Parse error", nil)
		return
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil || envelope == nil || envelope["result"] != nil || envelope["error"] != nil || request.JSONRPC != jsonrpc.Version || request.Method == "" {
		h.writeStreamableError(w, http.StatusBadRequest, request.ID, jsonrpc.ErrorCodeInvalidRequest,
			"Request body must contain one JSON-RPC request", nil)
		return
	}
	if !request.IsNotification() && !validJSONRPCID(request.ID) {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeInvalidRequest,
			"JSON-RPC id must be a string or safe integer", nil)
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
	if request.Method == "subscriptions/listen" {
		h.serveSubscriptionsListen(w, r, &request)
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
	if response.Error != nil {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			h.logger.Error("Failed to encode Streamable HTTP error response", "error", err)
		}
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

func validJSONRPCID(id any) bool {
	switch value := id.(type) {
	case string:
		return true
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value && math.Abs(value) <= 1<<53-1
	default:
		return false
	}
}

func validateUniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("JSON object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("request body contains more than one JSON value")
		}
		return err
	}
	return nil
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
	case "tools/call":
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
	bindings, annotationError := collectToolHeaderBindings(tool.Schema(), nil, nil)
	if annotationError != "" {
		return annotationError
	}
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

func collectToolHeaderBindings(schema map[string]any, path []string, seen map[string]struct{}) ([]toolHeaderBinding, string) {
	if seen == nil {
		seen = make(map[string]struct{})
	}
	properties, _ := schema["properties"].(map[string]any)
	bindings := make([]toolHeaderBinding, 0)
	for propertyName, raw := range properties {
		property, _ := raw.(map[string]any)
		propertyPath := append(append([]string(nil), path...), propertyName)
		if rawAnnotation, exists := property["x-mcp-header"]; exists {
			annotation, ok := rawAnnotation.(string)
			if !ok || !validHTTPToken(annotation) {
				return nil, "tool schema contains an invalid x-mcp-header annotation"
			}
			valueType, _ := property["type"].(string)
			if valueType != "string" && valueType != "integer" && valueType != "boolean" {
				return nil, "tool schema uses x-mcp-header on an unsupported parameter type"
			}
			key := strings.ToLower(annotation)
			if _, duplicate := seen[key]; duplicate {
				return nil, "tool schema contains duplicate x-mcp-header annotations"
			}
			seen[key] = struct{}{}
			bindings = append(bindings, toolHeaderBinding{
				headerName: annotation,
				path:       propertyPath,
				valueType:  valueType,
			})
		}
		nested, annotationError := collectToolHeaderBindings(property, propertyPath, seen)
		if annotationError != "" {
			return nil, annotationError
		}
		bindings = append(bindings, nested...)
	}
	return bindings, ""
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
	case "server/discover", "resources/list", "resources/templates/list", "resources/read", "tools/list", "tools/call", "subscriptions/listen":
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
			"resources": map[string]any{"subscribe": h.hasSubscribableResourceTemplates()},
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
