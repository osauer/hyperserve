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

	if got := len(engine.RegisteredMethods()); got != 0 {
		t.Errorf("expected no registered methods, got %d", got)
	}
}

func TestRegisterMethod(t *testing.T) {
	engine := NewEngine(nil)
	engine.RegisterMethod("test", func(params any) (any, error) { return "ok", nil })

	methods := engine.RegisteredMethods()
	if len(methods) != 1 || methods[0] != "test" {
		t.Fatalf("expected method list to contain 'test', got %v", methods)
	}
}

func TestProcessRequestValid(t *testing.T) {
	engine := NewEngine(nil)
	engine.RegisterMethod("echo", func(params any) (any, error) { return params, nil })

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

func TestProcessRequestNilResult(t *testing.T) {
	engine := NewEngine(nil)
	engine.RegisterMethod("void", func(any) (any, error) { return nil, nil })
	data := engine.ProcessRequest([]byte(`{"jsonrpc":"2.0","method":"void","id":1}`))
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire["result"]) != "null" || wire["error"] != nil || string(wire["id"]) != "1" {
		t.Fatalf("success must contain result:null and preserve id: %s", data)
	}
}

func TestResponseMarshalResultOrError(t *testing.T) {
	for _, tt := range []struct {
		name       string
		result     any
		err        *ErrorDetails
		wantResult string
	}{
		{"nil", nil, nil, "null"},
		{"typed nil", (*string)(nil), nil, "null"},
		{"false", false, nil, "false"},
		{"zero", 0, nil, "0"},
		{"empty string", "", nil, `""`},
		{"error", "ignored", &ErrorDetails{Code: ErrorCodeInternalError, Message: "failed"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := Response{JSONRPC: Version, Result: tt.result, Error: tt.err, ID: nil}
			for _, value := range []any{response, &response} {
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var wire map[string]json.RawMessage
				if err := json.Unmarshal(data, &wire); err != nil {
					t.Fatal(err)
				}
				if string(wire["result"]) != tt.wantResult || (wire["error"] != nil) != (tt.err != nil) || string(wire["id"]) != "null" {
					t.Fatalf("invalid result/error envelope: %s", data)
				}
			}
		})
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

func TestProcessRequestNotificationHasNoResponse(t *testing.T) {
	engine := NewEngine(nil)
	called := false
	engine.RegisterMethod("notify", func(params any) (any, error) {
		called = true
		return "ignored", nil
	})

	respRaw := engine.ProcessRequest([]byte(`{"jsonrpc":"2.0","method":"notify"}`))
	if len(respRaw) != 0 {
		t.Fatalf("notification response = %s, want empty", respRaw)
	}
	if !called {
		t.Fatal("notification handler was not called")
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
	engine.RegisterMethod("boom", func(params any) (any, error) { return nil, assertError("fail") })

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
