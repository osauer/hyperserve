package mcp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	jsonrpc "github.com/osauer/hyperserve/v2/pkg/jsonrpc"
)

// ErrMethodNotAllowed and ErrUnsupportedContentType are sentinel errors used by
// the HTTP transport so ServeHTTP can categorise failures with errors.Is
// rather than substring-matching free-form messages. Adding a wrap site is a
// public contract — keep the sentinel set narrow and stable.
var (
	ErrMethodNotAllowed       = errors.New("method not allowed")
	ErrUnsupportedContentType = errors.New("unsupported content type")
)

// Handler manages MCP protocol communication with multiple namespace support.
// The SSE state machine (client registry + per-client request channels) lives
// entirely in sseManager — Handler stays focused on JSON-RPC dispatch.
type Handler struct {
	tools                 map[string]Tool
	resources             map[string]Resource
	resourceTemplates     []resourceTemplateEntry
	resourceTemplateIndex map[string]int
	namespaces            map[string]*Namespace
	rpcEngine             *jsonrpc.Engine
	serverInfo            ServerInfo
	protocolVersion       string
	logger                *slog.Logger
	cache                 *resourceCache
	sseManager            *sseManager
	toolCallTimeout       time.Duration // Set via SetToolCallTimeout; defaults to 30s when zero.
	originValidator       func(*http.Request) bool
	legacyRoutedSSE       bool
	streamWriteTimeout    time.Duration
	streamKeepalive       time.Duration
	streamMu              sync.Mutex
	streams               map[*streamableSSE]struct{}
	shuttingDown          bool
}

type resourceTemplateEntry struct {
	uriTemplate string
	template    ResourceTemplate
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
		tools:                 make(map[string]Tool),
		resources:             make(map[string]Resource),
		resourceTemplateIndex: make(map[string]int),
		namespaces:            make(map[string]*Namespace),
		serverInfo:            serverInfo,
		protocolVersion:       DefaultProtocolVersion,
		logger:                logger,
		cache:                 newResourceCache(100),
		sseManager:            newSSEManager(),
		streamWriteTimeout:    streamableSSEWriteTimeout,
		streamKeepalive:       streamableSSEKeepalive,
		streams:               make(map[*streamableSSE]struct{}),
	}
	handler.rpcEngine = handler.newRPCEngine(nil)
	return handler
}

// ServerInfo returns the server info associated with this handler.
func (h *Handler) ServerInfo() ServerInfo { return h.serverInfo }

// ProtocolVersion returns the MCP protocol version this handler advertises.
func (h *Handler) ProtocolVersion() string { return h.protocolVersion }

// SetProtocolVersion overrides the initialize-era compatibility version
// advertised in legacy responses and discovery. Empty values reset to the
// default. StreamableHTTPProtocolVersion is selected independently through
// per-request metadata and cannot be configured as the legacy version.
func (h *Handler) SetProtocolVersion(version string) {
	if strings.TrimSpace(version) == "" {
		h.protocolVersion = DefaultProtocolVersion
		return
	}
	if version == StreamableHTTPProtocolVersion {
		h.logger.Warn("Current Streamable HTTP version cannot be used as the initialize-era compatibility version",
			"version", version,
			"fallback", DefaultProtocolVersion)
		h.protocolVersion = DefaultProtocolVersion
		return
	}
	h.protocolVersion = version
}

// ToolCount returns the number of registered tools.
func (h *Handler) ToolCount() int { return len(h.tools) }

// ResourceCount returns the number of registered resources.
func (h *Handler) ResourceCount() int { return len(h.resources) }

// ResourceTemplateCount returns the number of registered resource templates.
func (h *Handler) ResourceTemplateCount() int { return len(h.resourceTemplates) }

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

// HasResourceTemplate reports whether a resource template is registered.
func (h *Handler) HasResourceTemplate(uriTemplate string) bool {
	_, ok := h.resourceTemplateIndex[uriTemplate]
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
		l = logger
	}
	h.logger = l
	if h.rpcEngine != nil {
		h.rpcEngine.SetLogger(l)
	}
}

// SetOriginValidator overrides the default MCP Origin policy. The default
// accepts requests without Origin (normal for non-browser clients) and
// requires browser origins to match the request Host. Passing nil restores
// that default. Applications allowing cross-origin browser clients should
// authenticate them and validate an explicit origin allowlist here.
func (h *Handler) SetOriginValidator(validator func(*http.Request) bool) {
	h.originValidator = validator
}

// SetLegacyRoutedSSEEnabled enables HyperServe's proprietary X-SSE-* routed
// stream. It is disabled by default and exists only for migration of existing
// HyperServe-specific clients.
//
// Deprecated: use MCP 2026-07-28 Streamable HTTP and subscriptions/listen.
func (h *Handler) SetLegacyRoutedSSEEnabled(enabled bool) {
	h.legacyRoutedSSE = enabled
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

// RegisterResourceTemplate registers an MCP resource template without
// namespace prefixing.
func (h *Handler) RegisterResourceTemplate(template ResourceTemplate) {
	uriTemplate := template.URITemplate()
	if idx, exists := h.resourceTemplateIndex[uriTemplate]; exists {
		h.resourceTemplates[idx] = resourceTemplateEntry{uriTemplate: uriTemplate, template: template}
	} else {
		h.resourceTemplateIndex[uriTemplate] = len(h.resourceTemplates)
		h.resourceTemplates = append(h.resourceTemplates, resourceTemplateEntry{uriTemplate: uriTemplate, template: template})
	}
	h.logger.Debug("MCP resource template registered", "resource", template.Name(), "uriTemplate", uriTemplate)
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

// RegisterResourceTemplateInNamespace registers an MCP resource template in
// the specified namespace.
func (h *Handler) RegisterResourceTemplateInNamespace(template ResourceTemplate, namespace string) {
	if namespace == "" {
		h.logger.Error("Cannot register resource template without namespace", "resource", template.Name())
		return
	}
	wrapped := &namespacedResourceTemplate{
		namespace: namespace,
		prefix:    h.formatResourceName(namespace, ""),
		template:  template,
	}
	if subscribable, ok := template.(SubscribableResourceTemplate); ok {
		h.RegisterResourceTemplate(&namespacedSubscribableResourceTemplate{
			namespacedResourceTemplate: wrapped,
			template:                   subscribable,
		})
		return
	}
	h.RegisterResourceTemplate(wrapped)
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
	for _, template := range ns.ResourceTemplates {
		h.RegisterResourceTemplateInNamespace(template, name)
	}
	h.namespaces[name] = ns
	h.logger.Debug("MCP namespace registered", "namespace", name, "tools", len(ns.Tools), "resources", len(ns.Resources), "resourceTemplates", len(ns.ResourceTemplates))
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

// RegisteredResourceTemplates returns all registered resource template URI
// templates in registration order.
func (h *Handler) RegisteredResourceTemplates() []string {
	templates := make([]string, 0, len(h.resourceTemplates))
	for _, entry := range h.resourceTemplates {
		templates = append(templates, entry.uriTemplate)
	}
	return templates
}

// Tool returns a tool by its (possibly prefixed) name.
func (h *Handler) Tool(name string) (Tool, bool) {
	tool, exists := h.tools[name]
	return tool, exists
}

// Capabilities returns the server's MCP capabilities.
func (h *Handler) Capabilities() Capabilities {
	capabilities := Capabilities{
		Resources: &ResourcesCapability{Subscribe: h.hasSubscribableResourceTemplates(), ListChanged: false},
		Tools:     &ToolsCapability{ListChanged: false},
	}
	if h.legacyRoutedSSE {
		capabilities.SSE = &SSECapability{
			Enabled:       true,
			Endpoint:      "same",
			HeaderRouting: true,
		}
	}
	return capabilities
}

func (h *Handler) hasSubscribableResourceTemplates() bool {
	for _, entry := range h.resourceTemplates {
		if _, ok := entry.template.(SubscribableResourceTemplate); ok {
			return true
		}
	}
	return false
}

// ProcessRequest processes a single MCP request (raw JSON).
func (h *Handler) ProcessRequest(requestData []byte) []byte {
	return h.rpcEngine.ProcessRequest(requestData)
}

func (h *Handler) newRPCEngine(session *mcpSession) *jsonrpc.Engine {
	engine := jsonrpc.NewEngine(h.logger)
	h.registerMCPMethods(engine, session)
	return engine
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

// isSSEAccepted reports whether the Accept header asks for an SSE stream.
func isSSEAccepted(accept string) bool {
	if accept == "" {
		return false
	}
	for part := range strings.SplitSeq(strings.ToLower(accept), ",") {
		mediaType, _, _ := strings.Cut(part, ";")
		if strings.TrimSpace(mediaType) == "text/event-stream" {
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
	if !h.isOriginAllowed(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if h.isShuttingDown() {
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}

	// The 2026-07-28 transport is selected by its per-request protocol
	// metadata. Requests without those headers retain the initialize-era HTTP
	// behavior, which gives existing 2025 clients an explicit compatibility
	// path on the same endpoint.
	if r.Method == http.MethodPost && h.isStreamableHTTPRequest(r) {
		h.serveStreamableHTTP(w, r)
		return
	}

	if h.legacyRoutedSSE {
		// The proprietary standalone stream is GET-only. It is reachable only
		// after an application explicitly opts into the compatibility mode.
		if r.Method == http.MethodGet && isSSEAccepted(r.Header.Get("Accept")) {
			h.sseManager.HandleSSE(w, r, h)
			return
		}

		// Preserve the old human/status GET surface together with the legacy
		// transport; standards-only endpoints reject every non-POST method.
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
				}
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			path := html.EscapeString(r.URL.Path)
			fmt.Fprintf(w, htmlHelpTemplate, path, h.protocolVersion, path, path, path, path)
			return
		}

		if clientID := r.Header.Get("X-SSE-Client-ID"); clientID != "" {
			h.handleSSERoutedRequest(w, r, clientID)
			return
		}
	} else if hasLegacyRoutedSSEHeaders(r.Header) {
		http.Error(w, "Legacy routed SSE is disabled", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	transport := newHTTPTransport(w, r, h.logger)
	defer transport.Close()

	if err := h.ProcessRequestWithTransport(transport); err != nil {
		h.logger.Error("Failed to process MCP request", "error", err)
		switch {
		case errors.Is(err, ErrMethodNotAllowed):
			http.Error(w, "Method not allowed. MCP requires POST requests.", http.StatusMethodNotAllowed)
		case errors.Is(err, ErrUnsupportedContentType):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// ProcessRequestWithTransport processes an MCP request using the provided transport.
func (h *Handler) ProcessRequestWithTransport(transport Transport) error {
	return h.processRequestWithTransportAndSession(transport, nil, h.rpcEngine)
}

func (h *Handler) processRequestWithTransportAndSession(transport Transport, session *mcpSession, engine *jsonrpc.Engine) error {
	request, err := transport.Receive()
	if err != nil {
		return fmt.Errorf("failed to receive request: %w", err)
	}

	return h.processRequestObjectWithSession(request, transport, session, engine)
}

func (h *Handler) processRequestObjectWithSession(request *jsonrpc.Request, transport Transport, session *mcpSession, engine *jsonrpc.Engine) error {
	response := engine.ProcessRequestDirect(request)
	if response == nil {
		if transport, ok := transport.(interface{ NoResponse() error }); ok {
			if err := transport.NoResponse(); err != nil {
				return err
			}
		}
		if session != nil {
			session.startPending()
		}
		return nil
	}

	if err := transport.Send(response); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	if session != nil {
		session.startPending()
	}
	return nil
}

func (h *Handler) registerMCPMethods(engine *jsonrpc.Engine, session *mcpSession) {
	engine.RegisterMethod("initialize", h.handleInitialize)
	engine.RegisterMethod("initialized", h.handleInitialized)
	engine.RegisterMethod("resources/list", h.handleResourcesList)
	engine.RegisterMethod("resources/templates/list", h.handleResourceTemplatesList)
	engine.RegisterMethod("resources/read", func(params any) (any, error) {
		return h.handleResourcesRead(session, params)
	})
	engine.RegisterMethod("resources/subscribe", func(params any) (any, error) {
		return h.handleResourcesSubscribe(session, params)
	})
	engine.RegisterMethod("resources/unsubscribe", func(params any) (any, error) {
		return h.handleResourcesUnsubscribe(session, params)
	})
	engine.RegisterMethod("tools/list", h.handleToolsList)
	engine.RegisterMethod("tools/call", func(params any) (any, error) {
		ctx := context.Background()
		if session != nil {
			ctx = session.ctx
		}
		return h.handleToolsCallContext(ctx, params)
	})
	engine.RegisterMethod("ping", h.handlePing)
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
	instructions := "After receiving this response, send an 'initialized' notification. For live HTTP updates, use MCP 2026-07-28 subscriptions/listen."
	if h.legacyRoutedSSE {
		instructions += " Deprecated routed SSE is enabled temporarily; connect to the same endpoint with 'Accept: text/event-stream'."
	}

	return map[string]any{
		"protocolVersion": h.protocolVersion,
		"capabilities":    h.Capabilities(),
		"serverInfo":      h.serverInfo,
		"instructions":    instructions,
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

func (h *Handler) handleResourceTemplatesList(_ any) (any, error) {
	templates := make([]ResourceTemplateInfo, 0, len(h.resourceTemplates))
	for _, entry := range h.resourceTemplates {
		template := entry.template
		templates = append(templates, ResourceTemplateInfo{
			URITemplate: entry.uriTemplate,
			Name:        template.Name(),
			Description: template.Description(),
			MimeType:    template.MimeType(),
		})
	}
	return map[string]any{"resourceTemplates": templates}, nil
}

func (h *Handler) handleResourcesRead(session *mcpSession, params any) (any, error) {
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
	if exists {
		return h.readStaticResource(readParams.URI, resource)
	}

	template, templateParams, ok := h.matchResourceTemplate(readParams.URI)
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", readParams.URI)
	}

	ctx := context.Background()
	if session != nil {
		ctx = session.ctx
	}
	content, err := template.Read(ctx, readParams.URI, templateParams)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource template: %w", err)
	}
	contentBlock, err := resourceContent(readParams.URI, template.MimeType(), content)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"contents": []ResourceContent{contentBlock},
	}, nil
}

func (h *Handler) readStaticResource(uri string, resource Resource) (any, error) {
	cacheKey := uri
	cacheTTL := resourceCacheTTL(resource)
	if cacheTTL > 0 {
		if cachedContent, hit := h.cache.get(cacheKey); hit {
			return map[string]any{
				"contents": []ResourceContent{
					{URI: uri, MimeType: resource.MimeType(), Text: cachedContent},
				},
			}, nil
		}
	}

	content, err := resource.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}

	contentBlock, err := resourceContent(uri, resource.MimeType(), content)
	if err != nil {
		return nil, err
	}

	if cacheTTL > 0 {
		h.cache.set(cacheKey, contentBlock.Text, cacheTTL)
	}

	return map[string]any{
		"contents": []ResourceContent{contentBlock},
	}, nil
}

func (h *Handler) matchResourceTemplate(uri string) (ResourceTemplate, map[string]string, bool) {
	for _, entry := range h.resourceTemplates {
		params, ok := entry.template.Match(uri)
		if ok {
			return entry.template, params, true
		}
	}
	return nil, nil, false
}

func (h *Handler) matchSubscribableResourceTemplate(uri string) (SubscribableResourceTemplate, map[string]string, bool) {
	template, params, ok := h.matchResourceTemplate(uri)
	if !ok {
		return nil, nil, false
	}
	subscribable, ok := template.(SubscribableResourceTemplate)
	return subscribable, params, ok
}

func (h *Handler) handleResourcesSubscribe(session *mcpSession, params any) (any, error) {
	if session == nil {
		return nil, fmt.Errorf("resources/subscribe requires a live MCP session (SSE or stdio)")
	}
	var subscribeParams ResourceReadParams
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		if err := json.Unmarshal(paramBytes, &subscribeParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal subscribe params: %w", err)
		}
	}
	if subscribeParams.URI == "" {
		return nil, fmt.Errorf("uri parameter is required for resources/subscribe method")
	}
	template, templateParams, ok := h.matchSubscribableResourceTemplate(subscribeParams.URI)
	if !ok {
		return nil, fmt.Errorf("subscribable resource not found: %s", subscribeParams.URI)
	}
	if err := session.subscribe(subscribeParams.URI, template, templateParams); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

func (h *Handler) handleResourcesUnsubscribe(session *mcpSession, params any) (any, error) {
	if session == nil {
		return nil, fmt.Errorf("resources/unsubscribe requires a live MCP session (SSE or stdio)")
	}
	var unsubscribeParams ResourceReadParams
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		if err := json.Unmarshal(paramBytes, &unsubscribeParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal unsubscribe params: %w", err)
		}
	}
	if unsubscribeParams.URI == "" {
		return nil, fmt.Errorf("uri parameter is required for resources/unsubscribe method")
	}
	session.unsubscribe(unsubscribeParams.URI)
	return map[string]any{}, nil
}

func resourceCacheTTL(resource Resource) time.Duration {
	cacheable, ok := resource.(CacheableResource)
	if !ok {
		return 0
	}
	return max(cacheable.ResourceCacheTTL(), 0)
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
	return h.handleToolsCallContext(context.Background(), params)
}

func (h *Handler) handleToolsCallContext(parent context.Context, params any) (any, error) {
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
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	result, err := ctxTool.ExecuteWithContext(ctx, callParams.Arguments)
	if err != nil {
		if toolErr, ok := errors.AsType[*toolError](err); ok {
			return ToolResult{
				Content: []map[string]any{{"type": "text", "text": toolErr.Error()}},
				IsError: true,
			}, nil
		}
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
// consolidated into sseManager admission and request-channel ownership as the
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
        <strong>Note:</strong> MCP protocol traffic uses JSON-RPC 2.0 over HTTP POST.
        This GET response is documentation only.
    </div>

    <h2>How to Use</h2>

    <div class="example">
        <h3>Discover MCP 2026-07-28</h3>
        <pre>curl -X POST %s \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: server/discover" \
  -d '{
    "jsonrpc": "2.0",
    "method": "server/discover",
    "params": {
      "_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28"}
    },
    "id": 1
  }'</pre>
    </div>

    <div class="example">
        <h3>Legacy 2025 Compatibility</h3>
        <p>Requests without 2026 per-request headers use the initialize-era
        compatibility protocol (configured version: <code>%s</code>).</p>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}'</pre>
    </div>

    <div class="example">
        <h3>List Available Tools (2026-07-28)</h3>
        <pre>curl -X POST %s \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/list" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":2}'</pre>
    </div>

    <h2>Available Methods</h2>
    <ul>
        <li><code>server/discover</code> - Discover the current stateless protocol</li>
        <li><code>initialize</code> - Initialize the legacy 2025 compatibility protocol</li>
        <li><code>ping</code> - Test legacy protocol connectivity</li>
        <li><code>tools/list</code> - List available tools</li>
        <li><code>tools/call</code> - Execute a tool</li>
        <li><code>resources/list</code> - List available resources</li>
        <li><code>resources/templates/list</code> - List available resource templates</li>
        <li><code>resources/read</code> - Read a resource</li>
        <li><code>resources/subscribe</code> - Subscribe to a resource over SSE or stdio</li>
        <li><code>resources/unsubscribe</code> - Cancel a resource subscription</li>
    </ul>

    <h2>Legacy HyperServe Routed SSE</h2>
    <p>This proprietary compatibility stream is not MCP Streamable HTTP.
    Connect with <code>Accept: text/event-stream</code>; the
    initial <code>connection</code> event delivers a <code>clientId</code>
    and a <code>bindingToken</code>. Routed POSTs must echo BOTH:</p>
    <ul>
        <li>SSE stream: <code>GET %s</code> with header <code>Accept: text/event-stream</code></li>
        <li>Routed requests: <code>POST %s</code> with headers
            <code>X-SSE-Client-ID: {clientId}</code> and
            <code>X-SSE-Binding: {bindingToken}</code> (both required — missing or wrong binding returns 403)</li>
    </ul>

    <h2>More Information</h2>
    <p>For detailed documentation, see the <a href="https://github.com/osauer/hyperserve">HyperServe GitHub repository</a>.</p>
</body>
</html>`
