package mcp

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

// Handler manages MCP protocol communication with multiple namespace support.
// The SSE state machine (client registry + per-client request channels) lives
// entirely in sseManager — Handler stays focused on JSON-RPC dispatch.
type Handler struct {
	tools           map[string]Tool
	resources       map[string]Resource
	namespaces      map[string]*Namespace
	rpcEngine       *jsonrpc.Engine
	serverInfo      ServerInfo
	logger          *slog.Logger
	cache           *resourceCache
	sseManager      *sseManager
	toolCallTimeout time.Duration // Set via SetToolCallTimeout; defaults to 30s when zero.
}

// defaultToolCallTimeout is the per-tool execution budget. Tools that exceed
// this return context.DeadlineExceeded to the caller; see the caveat in
// contextToolWrapper.ExecuteWithContext for what happens to the goroutine.
const defaultToolCallTimeout = 30 * time.Second

// SetToolCallTimeout overrides the per-call timeout used when dispatching
// tools/call. Zero or negative values reset to defaultToolCallTimeout.
func (h *Handler) SetToolCallTimeout(d time.Duration) {
	if d <= 0 {
		h.toolCallTimeout = 0
		return
	}
	h.toolCallTimeout = d
}

// NewHandler creates a new MCP handler instance.
func NewHandler(serverInfo ServerInfo) *Handler {
	handler := &Handler{
		tools:      make(map[string]Tool),
		resources:  make(map[string]Resource),
		namespaces: make(map[string]*Namespace),
		rpcEngine:  jsonrpc.NewEngine(logger),
		serverInfo: serverInfo,
		logger:     logger,
		cache:      newResourceCache(100),
		sseManager: newSSEManager(),
	}
	handler.registerMCPMethods()
	return handler
}

// ServerInfo returns the server info associated with this handler.
func (h *Handler) ServerInfo() ServerInfo { return h.serverInfo }

// ToolCount returns the number of registered tools.
func (h *Handler) ToolCount() int { return len(h.tools) }

// ResourceCount returns the number of registered resources.
func (h *Handler) ResourceCount() int { return len(h.resources) }

// HasTool reports whether a tool with the given (possibly prefixed) name is
// registered.
func (h *Handler) HasTool(name string) bool {
	_, ok := h.tools[name]
	return ok
}

// HasResource reports whether a resource with the given URI is registered.
func (h *Handler) HasResource(uri string) bool {
	_, ok := h.resources[uri]
	return ok
}

// RPCEngine returns the underlying JSON-RPC engine. Exposed so tests and
// transports can dispatch a parsed request without re-marshalling.
func (h *Handler) RPCEngine() *jsonrpc.Engine { return h.rpcEngine }

// HasNamespace reports whether a namespace with the given name has been
// registered via RegisterNamespace.
func (h *Handler) HasNamespace(name string) bool {
	_, ok := h.namespaces[name]
	return ok
}

// Logger returns the handler's logger.
func (h *Handler) Logger() *slog.Logger { return h.logger }

// SetLogger overrides the handler's logger. Useful for tests that want to
// silence output.
func (h *Handler) SetLogger(l *slog.Logger) {
	if l == nil {
		h.logger = logger
		return
	}
	h.logger = l
}

func (h *Handler) formatToolName(namespace, toolName string) string {
	return fmt.Sprintf("mcp__%s__%s", namespace, toolName)
}

func (h *Handler) formatResourceName(namespace, resourceName string) string {
	return fmt.Sprintf("mcp__%s__%s", namespace, resourceName)
}

// RegisterTool registers an MCP tool without namespace prefixing.
func (h *Handler) RegisterTool(tool Tool) {
	h.tools[tool.Name()] = tool
	h.logger.Debug("MCP tool registered", "tool", tool.Name())
}

// RegisterToolInNamespace registers an MCP tool in the specified namespace.
func (h *Handler) RegisterToolInNamespace(tool Tool, namespace string) {
	namespace = cmp.Or(namespace, h.serverInfo.Name)
	prefixedName := h.formatToolName(namespace, tool.Name())
	h.tools[prefixedName] = tool
	h.logger.Debug("MCP tool registered in namespace", "tool", tool.Name(), "namespace", namespace, "prefixedName", prefixedName)
}

// RegisterResource registers an MCP resource without namespace prefixing.
func (h *Handler) RegisterResource(resource Resource) {
	h.resources[resource.URI()] = resource
	h.logger.Debug("MCP resource registered", "resource", resource.Name(), "uri", resource.URI())
}

// RegisterResourceInNamespace registers an MCP resource in the specified namespace.
func (h *Handler) RegisterResourceInNamespace(resource Resource, namespace string) {
	if namespace == "" {
		h.logger.Error("Cannot register resource without namespace", "resource", resource.Name())
		return
	}
	prefixedURI := h.formatResourceName(namespace, resource.URI())
	h.resources[prefixedURI] = resource
	h.logger.Debug("MCP resource registered in namespace", "resource", resource.Name(), "namespace", namespace, "uri", resource.URI(), "prefixedURI", prefixedURI)
}

// RegisterNamespace registers an entire namespace with its tools and resources.
func (h *Handler) RegisterNamespace(name string, configs ...NamespaceConfig) error {
	if name == "" {
		return fmt.Errorf("namespace name cannot be empty")
	}
	ns := &Namespace{Name: name}
	for _, config := range configs {
		config(ns)
	}
	for _, tool := range ns.Tools {
		h.RegisterToolInNamespace(tool, name)
	}
	for _, resource := range ns.Resources {
		h.RegisterResourceInNamespace(resource, name)
	}
	h.namespaces[name] = ns
	h.logger.Debug("MCP namespace registered", "namespace", name, "tools", len(ns.Tools), "resources", len(ns.Resources))
	return nil
}

// RegisteredTools returns all registered tool names.
// Returns a non-nil slice even when no tools are registered.
func (h *Handler) RegisteredTools() []string {
	tools := make([]string, 0, len(h.tools))
	return slices.AppendSeq(tools, maps.Keys(h.tools))
}

// RegisteredResources returns all registered resource URIs.
// Returns a non-nil slice even when no resources are registered.
func (h *Handler) RegisteredResources() []string {
	resources := make([]string, 0, len(h.resources))
	return slices.AppendSeq(resources, maps.Keys(h.resources))
}

// Tool returns a tool by its (possibly prefixed) name.
func (h *Handler) Tool(name string) (Tool, bool) {
	tool, exists := h.tools[name]
	return tool, exists
}

// Capabilities returns the server's MCP capabilities.
func (h *Handler) Capabilities() Capabilities {
	return Capabilities{
		Resources: &ResourcesCapability{Subscribe: false, ListChanged: false},
		Tools:     &ToolsCapability{ListChanged: false},
		SSE: &SSECapability{
			Enabled:       true,
			Endpoint:      "same",
			HeaderRouting: true,
		},
	}
}

// ProcessRequest processes a single MCP request (raw JSON).
func (h *Handler) ProcessRequest(requestData []byte) []byte {
	return h.rpcEngine.ProcessRequest(requestData)
}

// isJSONAccepted reports whether the Accept header indicates JSON is acceptable.
func isJSONAccepted(accept string) bool {
	if accept == "" {
		return false
	}
	accept = strings.ToLower(accept)
	if accept == "*/*" {
		return true
	}
	for part := range strings.SplitSeq(accept, ",") {
		mediaType, _, _ := strings.Cut(part, ";")
		mediaType = strings.TrimSpace(mediaType)
		if mediaType == "application/json" || mediaType == "*/*" || mediaType == "application/*" {
			return true
		}
	}
	return false
}

// ServeHTTP implements http.Handler for the MCP endpoint.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.logger.Enabled(context.Background(), slog.LevelDebug) {
		h.logger.Debug("MCP ServeHTTP called", "path", r.URL.Path, "method", r.Method)
	}

	// SSE route via Accept header.
	if r.Header.Get("Accept") == "text/event-stream" {
		h.sseManager.HandleSSE(w, r, h)
		return
	}

	// GET requests get a helpful HTML or JSON status.
	if r.Method == http.MethodGet {
		accept := r.Header.Get("Accept")
		if isJSONAccepted(accept) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := map[string]any{
				"status":       "ready",
				"server":       h.serverInfo,
				"capabilities": h.Capabilities(),
				"endpoint":     r.URL.Path,
				"transport":    "http",
			}
			if err := json.NewEncoder(w).Encode(status); err != nil {
				h.logger.Error("Failed to encode JSON status", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, htmlHelpTemplate, r.URL.Path, r.URL.Path, r.URL.Path, r.URL.Path, r.URL.Path)
		return
	}

	// SSE-routed responses: client ID in header.
	if clientID := r.Header.Get("X-SSE-Client-ID"); clientID != "" {
		h.handleSSERoutedRequest(w, r, clientID)
		return
	}

	transport := newHTTPTransport(w, r, h.logger)
	defer transport.Close()

	if err := h.ProcessRequestWithTransport(transport); err != nil {
		h.logger.Error("Failed to process MCP request", "error", err)
		switch {
		case strings.Contains(err.Error(), "method not allowed"):
			http.Error(w, "Method not allowed. MCP requires POST requests.", http.StatusMethodNotAllowed)
		case strings.Contains(err.Error(), "Content-Type"):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// ProcessRequestWithTransport processes an MCP request using the provided transport.
func (h *Handler) ProcessRequestWithTransport(transport Transport) error {
	request, err := transport.Receive()
	if err != nil {
		return fmt.Errorf("failed to receive request: %w", err)
	}

	response := h.rpcEngine.ProcessRequestDirect(request)

	if err := transport.Send(response); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	return nil
}

func (h *Handler) registerMCPMethods() {
	h.rpcEngine.RegisterMethod("initialize", h.handleInitialize)
	h.rpcEngine.RegisterMethod("initialized", h.handleInitialized)
	h.rpcEngine.RegisterMethod("resources/list", h.handleResourcesList)
	h.rpcEngine.RegisterMethod("resources/read", h.handleResourcesRead)
	h.rpcEngine.RegisterMethod("tools/list", h.handleToolsList)
	h.rpcEngine.RegisterMethod("tools/call", h.handleToolsCall)
	h.rpcEngine.RegisterMethod("ping", h.handlePing)
}

func (h *Handler) handleInitialize(params any) (any, error) {
	var initParams InitializeParams
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		if err := json.Unmarshal(paramBytes, &initParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal init params: %w", err)
		}
	}
	h.logger.Debug("MCP client initialized", "client", initParams.ClientInfo.Name, "version", initParams.ClientInfo.Version)

	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    h.Capabilities(),
		"serverInfo":      h.serverInfo,
		"instructions":    "Follow the initialization protocol: after receiving this response, send an 'initialized' notification, then the server will send a 'ready' notification. For SSE support, connect to the SAME endpoint with 'Accept: text/event-stream' header.",
	}, nil
}

func (h *Handler) handleInitialized(_ any) (any, error) {
	h.logger.Debug("MCP client confirmed initialization")
	return nil, nil
}

func (h *Handler) handleResourcesList(_ any) (any, error) {
	resources := make([]ResourceInfo, 0, len(h.resources))
	for prefixedURI, resource := range h.resources {
		resources = append(resources, ResourceInfo{
			URI:         prefixedURI,
			Name:        resource.Name(),
			Description: resource.Description(),
			MimeType:    resource.MimeType(),
		})
	}
	return map[string]any{"resources": resources}, nil
}

func (h *Handler) handleResourcesRead(params any) (any, error) {
	var readParams ResourceReadParams

	if params != nil {
		// "arguments" is a tools/call concept; resources/read takes "uri".
		// Catching the mismatch up front gives a clearer error than letting
		// json.Unmarshal succeed-with-empty-fields and then "uri is required".
		if paramsMap, ok := params.(map[string]any); ok {
			if _, hasArguments := paramsMap["arguments"]; hasArguments {
				return nil, fmt.Errorf("invalid parameters: resources/read expects 'uri' parameter, not 'arguments'. Use tools/call for tool execution")
			}
		}

		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		if h.logger.Enabled(context.Background(), slog.LevelDebug) {
			h.logger.Debug("MCP resources/read parameters received", "params", string(paramBytes))
		}
		if err := json.Unmarshal(paramBytes, &readParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal read params: %w", err)
		}
	}

	if readParams.URI == "" {
		return nil, fmt.Errorf("uri parameter is required for resources/read method")
	}

	resource, exists := h.resources[readParams.URI]
	if !exists {
		return nil, fmt.Errorf("resource not found: %s", readParams.URI)
	}

	cacheKey := readParams.URI
	if cachedContent, hit := h.cache.get(cacheKey); hit {
		return map[string]any{
			"contents": []ResourceContent{
				{URI: resource.URI(), MimeType: resource.MimeType(), Text: cachedContent},
			},
		}, nil
	}

	content, err := resource.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}

	var textContent string
	switch v := content.(type) {
	case string:
		textContent = v
	case []byte:
		textContent = string(v)
	default:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource content to JSON: %w", err)
		}
		textContent = string(jsonBytes)
	}

	h.cache.set(cacheKey, textContent, 5*time.Minute)

	return map[string]any{
		"contents": []ResourceContent{
			{URI: resource.URI(), MimeType: resource.MimeType(), Text: textContent},
		},
	}, nil
}

func (h *Handler) handleToolsList(_ any) (any, error) {
	tools := make([]ToolInfo, 0, len(h.tools))
	for prefixedName, tool := range h.tools {
		info := ToolInfo{
			Name:        prefixedName,
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		}
		if out, ok := tool.(ToolWithOutputSchema); ok {
			info.OutputSchema = out.OutputSchema()
		}
		tools = append(tools, info)
	}
	return map[string]any{"tools": tools}, nil
}

func (h *Handler) handleToolsCall(params any) (any, error) {
	var callParams ToolCallParams

	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		if err := json.Unmarshal(paramBytes, &callParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal call params: %w", err)
		}
	}

	tool, exists := h.tools[callParams.Name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", callParams.Name)
	}

	timeout := h.toolCallTimeout
	if timeout <= 0 {
		timeout = defaultToolCallTimeout
	}
	ctxTool := wrapToolWithContext(tool)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := ctxTool.ExecuteWithContext(ctx, callParams.Arguments)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	content, err := toToolContent(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool response: %w", err)
	}

	response := ToolResult{Content: content}
	if resultMap, ok := result.(map[string]any); ok {
		if isError, ok := resultMap["isError"].(bool); ok && isError {
			response.IsError = true
		}
	}
	return response, nil
}

// toToolContent normalises a tool's `any` return into the MCP `content[]`
// shape. The supported inputs, in order:
//
//   - `string` — wrapped as a single text content frame.
//   - `map[string]any` with `content: []map[string]any` — passed through
//     (the tool already produced MCP content).
//   - `map[string]any` with `content: []any` whose every element is a
//     `map[string]any` — same as above after the element cast.
//   - anything else — JSON-marshalled into a single text content frame.
//
// The prior shape had a sub-branch that silently dropped a `json.Marshal`
// error and overwrote the in-progress content slice with a single text
// frame, leaving consumers no signal that something went wrong.
func toToolContent(result any) ([]map[string]any, error) {
	switch v := result.(type) {
	case string:
		return []map[string]any{{"type": "text", "text": v}}, nil

	case map[string]any:
		if existing, ok := v["content"].([]map[string]any); ok {
			return existing, nil
		}
		if existing, ok := v["content"].([]any); ok {
			coerced := make([]map[string]any, 0, len(existing))
			for _, item := range existing {
				m, ok := item.(map[string]any)
				if !ok {
					// Heterogeneous content slice — fall through to
					// "marshal the whole map as one text frame" rather
					// than silently produce a partial result.
					return marshalAsText(v)
				}
				coerced = append(coerced, m)
			}
			return coerced, nil
		}
		return marshalAsText(v)

	default:
		return marshalAsText(v)
	}
}

func marshalAsText(v any) ([]map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return []map[string]any{{"type": "text", "text": string(b)}}, nil
}

func (h *Handler) handlePing(_ any) (any, error) {
	return map[string]any{"message": "pong"}, nil
}

// handleSSERoutedRequest handles HTTP POSTs whose responses must be delivered
// over a previously-established SSE connection. The caller must present the
// X-SSE-Binding header that was issued in the connection event; mere
// possession of the client ID is not sufficient. This closes the
// session-injection class of attacks where a leaked or guessed ID could be
// used to inject requests into another client's stream.
func (h *Handler) handleSSERoutedRequest(w http.ResponseWriter, r *http.Request, clientID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	// Verify the supplied binding token before we read or queue anything.
	// A wrong/missing token returns 403 — indistinguishable from "no such
	// client" to avoid leaking which IDs are active.
	supplied := r.Header.Get("X-SSE-Binding")
	client, ok := h.sseManager.lookup(clientID)
	if !ok || !client.VerifyBinding(supplied) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	requestChan, exists := h.sseManager.requestChanFor(clientID)
	if !exists {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var request jsonrpc.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	select {
	case requestChan <- &request:
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "accepted",
			"message": "Request queued for processing",
		})
	default:
		http.Error(w, "Request queue full", http.StatusServiceUnavailable)
	}
}

// (Previously: RegistersseClient/UnregistersseClient/SendSSENotification.
// SendSSENotification had zero callers; the Register/Unregister pair was
// moved into sseManager.registerRequestChan/unregisterRequestChan as the
// SSE state machine now lives entirely in one place.)

const htmlHelpTemplate = `<!DOCTYPE html>
<html>
<head>
    <title>MCP Endpoint - HyperServe</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
               max-width: 800px; margin: 50px auto; padding: 20px; line-height: 1.6; }
        h1 { color: #333; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; }
        .example { margin: 20px 0; }
        .note { background: #e8f4f8; padding: 15px; border-left: 4px solid #0084c7; margin: 20px 0; }
    </style>
</head>
<body>
    <h1>Model Context Protocol (MCP) Endpoint</h1>

    <p>This endpoint implements the <a href="https://modelcontextprotocol.io">Model Context Protocol</a>
    for AI assistant integration.</p>

    <div class="note">
        <strong>Note:</strong> MCP uses JSON-RPC 2.0 over HTTP POST. GET requests are not supported.
    </div>

    <h2>How to Use</h2>

    <div class="example">
        <h3>Initialize Connection</h3>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "test-client", "version": "1.0.0"}
    },
    "id": 1
  }'</pre>
    </div>

    <div class="example">
        <h3>List Available Tools</h3>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "tools/list", "id": 2}'</pre>
    </div>

    <div class="example">
        <h3>List Available Resources</h3>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "resources/list", "id": 3}'</pre>
    </div>

    <h2>Available Methods</h2>
    <ul>
        <li><code>initialize</code> - Initialize MCP session</li>
        <li><code>ping</code> - Test connectivity</li>
        <li><code>tools/list</code> - List available tools</li>
        <li><code>tools/call</code> - Execute a tool</li>
        <li><code>resources/list</code> - List available resources</li>
        <li><code>resources/read</code> - Read a resource</li>
    </ul>

    <h2>Server-Sent Events (SSE) Support</h2>
    <p>This server also supports SSE for real-time communication:</p>
    <ul>
        <li>SSE endpoint: <code>%s/sse</code></li>
        <li>Send requests to <code>%s</code> with header <code>X-SSE-Client-ID: {your-client-id}</code></li>
        <li>Responses will be delivered via the SSE connection</li>
    </ul>

    <h2>More Information</h2>
    <p>For detailed documentation, see the <a href="https://github.com/osauer/hyperserve">HyperServe GitHub repository</a>.</p>
</body>
</html>`
