package mcpconformance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	hyperservemcp "github.com/osauer/hyperserve/v2/mcp"
)

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "returns its input" }
func (echoTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string"},
		},
		"required": []string{"message"},
	}
}
func (echoTool) Execute(params map[string]any) (any, error) { return params["message"], nil }

type liveResource struct {
	started  chan hyperservemcp.ResourceEmitter
	finished chan struct{}
}

func (r *liveResource) URITemplate() string { return "state://{name}" }
func (r *liveResource) Name() string        { return "state" }
func (r *liveResource) Description() string { return "live state" }
func (r *liveResource) MimeType() string    { return "application/json" }
func (r *liveResource) Match(uri string) (map[string]string, bool) {
	name, ok := strings.CutPrefix(uri, "state://")
	return map[string]string{"name": name}, ok && name != ""
}
func (r *liveResource) Read(context.Context, string, map[string]string) (any, error) {
	return map[string]any{"status": "ok"}, nil
}
func (r *liveResource) Subscribe(ctx context.Context, _ string, _ map[string]string, emit hyperservemcp.ResourceEmitter) error {
	r.started <- emit
	defer func() { r.finished <- struct{}{} }()
	<-ctx.Done()
	return ctx.Err()
}

type captureWriter struct {
	http.ResponseWriter
	once sync.Once
	ack  chan struct{}
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "notifications/subscriptions/acknowledged") {
		w.once.Do(func() { close(w.ack) })
	}
	return w.ResponseWriter.Write(p)
}

func (w *captureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *captureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestOfficialSDKStreamableHTTP(t *testing.T) {
	handler := hyperservemcp.NewHandler(hyperservemcp.ServerInfo{Name: "conformance", Version: "1.4.0"})
	handler.RegisterTool(echoTool{})
	type answer struct {
		Value  int               `json:"value"`
		Notes  []string          `json:"notes"`
		Next   *int              `json:"next"`
		Labels map[string]string `json:"labels"`
	}
	handler.RegisterTool(hyperservemcp.NewTypedTool("answer", "typed object", func(context.Context, struct{}) (answer, error) { return answer{Value: 42}, nil }))
	handler.RegisterTool(hyperservemcp.NewTypedTool("count", "typed scalar", func(context.Context, struct{}) (int, error) { return 42, nil }))
	handler.RegisterTool(hyperservemcp.NewTypedTool("answers", "typed array", func(context.Context, struct{}) ([]answer, error) { return []answer{{Value: 42}}, nil }))
	resource := &liveResource{
		started:  make(chan hyperservemcp.ResourceEmitter, 1),
		finished: make(chan struct{}, 1),
	}
	handler.RegisterResourceTemplate(resource)
	ack := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(&captureWriter{ResponseWriter: w, ack: ack}, r)
	}))
	defer server.Close()

	updates := make(chan string, 1)
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "hyperserve-conformance", Version: "1.0.0"},
		&sdkmcp.ClientOptions{
			ResourceUpdatedHandler: func(_ context.Context, request *sdkmcp.ResourceUpdatedNotificationRequest) {
				updates <- request.Params.URI
			},
		},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint:             server.URL,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("ListTools = %+v, %v", tools, err)
	}
	call, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"message": "hello"},
	})
	if err != nil || len(call.Content) != 1 {
		t.Fatalf("CallTool = %+v, %v", call, err)
	}
	content, ok := call.Content[0].(*sdkmcp.TextContent)
	if !ok || content.Text != "hello" {
		t.Fatalf("CallTool content = %#v", call.Content)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "echo" {
			continue
		}
		result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{}})
		if err != nil {
			t.Fatalf("CallTool(%s): %v", tool.Name, err)
		}
		if result.IsError || result.StructuredContent == nil || len(result.Content) != 1 {
			t.Fatalf("typed result = %+v", result)
		}
		schemaJSON, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(schemaJSON, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := resolved.Validate(result.StructuredContent); err != nil {
			t.Fatalf("%s output violates advertised schema: %v", tool.Name, err)
		}
		text, ok := result.Content[0].(*sdkmcp.TextContent)
		if !ok {
			t.Fatalf("typed content = %#v", result.Content)
		}
		var decoded any
		if err := json.Unmarshal([]byte(text.Text), &decoded); err != nil {
			t.Fatal(err)
		}
		if err := resolved.Validate(decoded); err != nil {
			t.Fatalf("%s text violates advertised schema: %v", tool.Name, err)
		}
	}

	if err := session.Subscribe(ctx, &sdkmcp.SubscribeParams{URI: "state://service"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	select {
	case <-ack:
	case <-ctx.Done():
		t.Fatal("official SDK did not receive a listen acknowledgement")
	}
	var emit hyperservemcp.ResourceEmitter
	select {
	case emit = <-resource.started:
	case <-ctx.Done():
		t.Fatal("resource producer did not start")
	}
	if err := emit.Update("state://service"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	select {
	case uri := <-updates:
		if uri != "state://service" {
			t.Fatalf("updated URI = %q", uri)
		}
	case <-ctx.Done():
		t.Fatal("official SDK did not receive the resource update")
	}
	if err := session.Unsubscribe(ctx, &sdkmcp.UnsubscribeParams{URI: "state://service"}); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	select {
	case <-resource.finished:
	case <-ctx.Done():
		t.Fatal("official SDK cancellation did not stop the resource producer")
	}

}
