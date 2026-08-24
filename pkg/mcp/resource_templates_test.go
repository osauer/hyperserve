package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	jsonrpc "github.com/osauer/hyperserve/v2/pkg/jsonrpc"
)

type quoteTemplate struct {
	readCalls int
	lastURI   string
}

func (t *quoteTemplate) URITemplate() string { return "quotes://{symbol}" }
func (t *quoteTemplate) Name() string        { return "Quotes" }
func (t *quoteTemplate) Description() string { return "Read quotes by symbol" }
func (t *quoteTemplate) MimeType() string    { return "application/json" }

func (t *quoteTemplate) Match(uri string) (map[string]string, bool) {
	symbol, ok := strings.CutPrefix(uri, "quotes://")
	if !ok || symbol == "" || strings.Contains(symbol, "/") {
		return nil, false
	}
	return map[string]string{"symbol": symbol}, true
}

func (t *quoteTemplate) Read(_ context.Context, uri string, params map[string]string) (any, error) {
	t.readCalls++
	t.lastURI = uri
	return map[string]any{
		"uri":    uri,
		"symbol": params["symbol"],
	}, nil
}

func TestHandlerResourceTemplatesListAndRead(t *testing.T) {
	h := newHandlerForTest(t)
	template := &quoteTemplate{}
	h.RegisterResourceTemplate(template)

	if h.ResourceTemplateCount() != 1 {
		t.Fatalf("ResourceTemplateCount = %d, want 1", h.ResourceTemplateCount())
	}
	if !h.HasResourceTemplate("quotes://{symbol}") {
		t.Fatal("HasResourceTemplate(quotes://{symbol}) = false")
	}
	if got := h.RegisteredResourceTemplates(); len(got) != 1 || got[0] != "quotes://{symbol}" {
		t.Fatalf("RegisteredResourceTemplates = %v, want [quotes://{symbol}]", got)
	}

	listResp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/templates/list",
		ID:      1,
	})
	if listResp.Error != nil {
		t.Fatalf("resources/templates/list error: %+v", listResp.Error)
	}
	listResult := listResp.Result.(map[string]any)
	templates := listResult["resourceTemplates"].([]ResourceTemplateInfo)
	if len(templates) != 1 {
		t.Fatalf("len(resourceTemplates) = %d, want 1", len(templates))
	}
	if templates[0].URITemplate != "quotes://{symbol}" || templates[0].MimeType != "application/json" {
		t.Fatalf("unexpected template info: %+v", templates[0])
	}

	readResp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/read",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      2,
	})
	if readResp.Error != nil {
		t.Fatalf("resources/read error: %+v", readResp.Error)
	}
	readResult := readResp.Result.(map[string]any)
	contents := readResult["contents"].([]ResourceContent)
	if len(contents) != 1 {
		t.Fatalf("len(contents) = %d, want 1", len(contents))
	}
	if contents[0].URI != "quotes://AAPL" {
		t.Fatalf("content URI = %q, want quotes://AAPL", contents[0].URI)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(contents[0].Text.(string)), &payload); err != nil {
		t.Fatalf("content text is not JSON: %v", err)
	}
	if payload["symbol"] != "AAPL" || payload["uri"] != "quotes://AAPL" {
		t.Fatalf("payload = %v, want AAPL raw URI", payload)
	}
}

func TestHandlerResourcesReadStaticResourcePrecedesTemplate(t *testing.T) {
	h := newHandlerForTest(t)
	template := &quoteTemplate{}
	h.RegisterResourceTemplate(template)
	h.RegisterResource(&stubResource{uri: "quotes://AAPL"})

	resp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/read",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      1,
	})
	if resp.Error != nil {
		t.Fatalf("resources/read error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	contents := result["contents"].([]ResourceContent)
	if contents[0].Text != "ok" {
		t.Fatalf("content text = %v, want static resource response", contents[0].Text)
	}
	if template.readCalls != 0 {
		t.Fatalf("template readCalls = %d, want 0", template.readCalls)
	}
}

func TestHandlerResourceTemplateNamespaceWrapsURI(t *testing.T) {
	h := newHandlerForTest(t)
	template := &quoteTemplate{}
	if err := h.RegisterNamespace("market", WithNamespaceResourceTemplates(template)); err != nil {
		t.Fatalf("RegisterNamespace: %v", err)
	}

	listResp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/templates/list",
		ID:      1,
	})
	if listResp.Error != nil {
		t.Fatalf("resources/templates/list error: %+v", listResp.Error)
	}
	templates := listResp.Result.(map[string]any)["resourceTemplates"].([]ResourceTemplateInfo)
	if len(templates) != 1 || templates[0].URITemplate != "mcp__market__quotes://{symbol}" {
		t.Fatalf("resourceTemplates = %+v, want namespaced quote template", templates)
	}

	readResp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/read",
		Params:  map[string]any{"uri": "mcp__market__quotes://MSFT"},
		ID:      2,
	})
	if readResp.Error != nil {
		t.Fatalf("resources/read error: %+v", readResp.Error)
	}
	contents := readResp.Result.(map[string]any)["contents"].([]ResourceContent)
	if contents[0].URI != "mcp__market__quotes://MSFT" {
		t.Fatalf("content URI = %q, want prefixed URI", contents[0].URI)
	}
	if template.lastURI != "quotes://MSFT" {
		t.Fatalf("template saw URI = %q, want raw URI", template.lastURI)
	}
}

func TestExtensionBuilderRegistersResourceTemplates(t *testing.T) {
	h := newHandlerForTest(t)
	ext := NewExtension("market").
		WithResourceTemplate(&quoteTemplate{}).
		Build()

	if err := h.RegisterExtension(ext); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}
	if !h.HasResourceTemplate("quotes://{symbol}") {
		t.Fatal("extension resource template was not registered")
	}
}

type responseCaptureTransport struct {
	responses chan *jsonrpc.Response
}

func newResponseCaptureTransport() *responseCaptureTransport {
	return &responseCaptureTransport{responses: make(chan *jsonrpc.Response, 4)}
}

func (t *responseCaptureTransport) Send(response *jsonrpc.Response) error {
	t.responses <- response
	return nil
}

func (t *responseCaptureTransport) Receive() (*jsonrpc.Request, error) { return nil, io.EOF }
func (t *responseCaptureTransport) Close() error                       { return nil }
