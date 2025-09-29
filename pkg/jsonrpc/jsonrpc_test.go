package jsonrpc

import (
	"encoding/json"
	"testing"
)

func TestNewEngine(t *testing.T) {
	engine := NewEngine(nil)
	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}

	if got := len(engine.GetRegisteredMethods()); got != 0 {
		t.Errorf("expected no registered methods, got %d", got)
	}
}

func TestRegisterMethod(t *testing.T) {
	engine := NewEngine(nil)
	engine.RegisterMethod("test", func(params interface{}) (interface{}, error) { return "ok", nil })

	methods := engine.GetRegisteredMethods()
	if len(methods) != 1 || methods[0] != "test" {
		t.Fatalf("expected method list to contain 'test', got %v", methods)
	}
}

func TestProcessRequestValid(t *testing.T) {
	engine := NewEngine(nil)
	engine.RegisterMethod("echo", func(params interface{}) (interface{}, error) { return params, nil })

	payload := Request{JSONRPC: Version, Method: "echo", Params: map[string]string{"k": "v"}, ID: 1}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	respRaw := engine.ProcessRequest(raw)
	var resp Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("expected no error, got %+v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatalf("expected result payload")
	}
}

func TestProcessRequestInvalidJSON(t *testing.T) {
	engine := NewEngine(nil)
	respRaw := engine.ProcessRequest([]byte("{"))
	var resp Response
	if err := json.Unmarshal(respRaw, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil || resp.Error.Code != ErrorCodeParseError {
		t.Fatalf("expected parse error, got %+v", resp.Error)
	}
}

func TestProcessRequestMethodNotFound(t *testing.T) {
	engine := NewEngine(nil)
	payload := Request{JSONRPC: Version, Method: "missing", ID: 1}
	raw, _ := json.Marshal(payload)

	respRaw := engine.ProcessRequest(raw)
	var resp Response
	_ = json.Unmarshal(respRaw, &resp)

	if resp.Error == nil || resp.Error.Code != ErrorCodeMethodNotFound {
		t.Fatalf("expected method not found, got %+v", resp.Error)
	}
}

func TestProcessRequestMethodError(t *testing.T) {
	engine := NewEngine(nil)
	engine.RegisterMethod("boom", func(params interface{}) (interface{}, error) { return nil, assertError("fail") })

	payload := Request{JSONRPC: Version, Method: "boom", ID: 1}
	raw, _ := json.Marshal(payload)

	respRaw := engine.ProcessRequest(raw)
	var resp Response
	_ = json.Unmarshal(respRaw, &resp)

	if resp.Error == nil || resp.Error.Code != ErrorCodeInternalError {
		t.Fatalf("expected internal error, got %+v", resp.Error)
	}
}

// assertError implements error for test assertions.
type assertError string

func (e assertError) Error() string { return string(e) }
