package builtin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"maps"
	"reflect"
	"testing"
	"testing/slogtest"
)

func capturedRecords(t *testing.T, resource *ServerLogResource) []map[string]any {
	t.Helper()
	data, err := resource.Read()
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Logs []map[string]any `json:"logs"`
	}
	if err := json.Unmarshal([]byte(data.(string)), &payload); err != nil {
		t.Fatal(err)
	}
	for _, record := range payload.Logs {
		attrs, _ := record["attrs"].(map[string]any)
		delete(record, "attrs")
		maps.Copy(record, attrs)
	}
	return payload.Logs
}

func TestServerLogResourceSlogContract(t *testing.T) {
	resource := NewServerLogResource(100)
	if err := slogtest.TestHandler(resource, func() []map[string]any {
		return capturedRecords(t, resource)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestServerLogResourceDerivedHandlersPreserveForwarding(t *testing.T) {
	var forwarded bytes.Buffer
	resource := NewServerLogResource(10)
	resource.handler = slog.NewJSONHandler(&forwarded, nil)
	base := slog.New(resource)
	child := base.With("request_id", "req-1").WithGroup("http").With("method", "GET")
	child.WithGroup("response").Info("handled", "status", 204)
	base.Info("unscoped", "service", "example")
	got := capturedRecords(t, resource)
	decoder := json.NewDecoder(&forwarded)
	for i, captured := range got {
		var sent map[string]any
		if err := decoder.Decode(&sent); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(captured, sent) {
			t.Errorf("record %d capture = %#v, forwarded = %#v", i, captured, sent)
		}
	}
	if got[0]["request_id"] != "req-1" || got[0]["http"].(map[string]any)["response"].(map[string]any)["status"] != float64(204) {
		t.Fatalf("missing scoped attributes: %#v", got[0])
	}
	if _, exists := got[1]["http"]; exists {
		t.Fatal("derived groups leaked into the parent logger")
	}
}

type changingLogValue struct {
	value string
}

func (v *changingLogValue) LogValue() slog.Value { return slog.StringValue(v.value) }

func TestServerLogResourceBoundLogValuesAreSnapshots(t *testing.T) {
	var forwarded bytes.Buffer
	resource := NewServerLogResource(10)
	resource.handler = slog.NewJSONHandler(&forwarded, nil)
	value := &changingLogValue{value: "bound"}
	logger := slog.New(resource).WithGroup("request").With(slog.Group("context", "value", value))
	value.value = "changed"
	logger.Info("handled")
	captured := capturedRecords(t, resource)[0]
	var sent map[string]any
	if err := json.Unmarshal(forwarded.Bytes(), &sent); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured, sent) {
		t.Errorf("capture = %#v, forwarded = %#v", captured, sent)
	}
	got := captured["request"].(map[string]any)["context"].(map[string]any)["value"]
	if got != "bound" {
		t.Errorf("bound log value = %v, want bound", got)
	}
}
