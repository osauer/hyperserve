package hyperserve

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONRPCEngine_NewEngine(t *testing.T) {
	engine := NewJSONRPCEngine()
	if engine == nil {
		t.Fatal("NewJSONRPCEngine returned nil")
	}
	
	if engine.methods == nil {
		t.Error("Engine methods map is nil")
	}
	
	if len(engine.methods) != 0 {
		t.Error("Engine should start with no registered methods")
	}
}

func TestJSONRPCEngine_RegisterMethod(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	// Register a test method
	testHandler := func(params interface{}) (interface{}, error) {
		return "test result", nil
	}
	
	engine.RegisterMethod("test", testHandler)
	
	if len(engine.methods) != 1 {
		t.Errorf("Expected 1 registered method, got %d", len(engine.methods))
	}
	
	if _, exists := engine.methods["test"]; !exists {
		t.Error("Test method not found in registered methods")
	}
}

func TestJSONRPCEngine_GetRegisteredMethods(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	// Initially should be empty
	methods := engine.GetRegisteredMethods()
	if len(methods) != 0 {
		t.Errorf("Expected 0 methods, got %d", len(methods))
	}
	
	// Register some methods
	engine.RegisterMethod("method1", func(params interface{}) (interface{}, error) { return nil, nil })
	engine.RegisterMethod("method2", func(params interface{}) (interface{}, error) { return nil, nil })
	
	methods = engine.GetRegisteredMethods()
	if len(methods) != 2 {
		t.Errorf("Expected 2 methods, got %d", len(methods))
	}
	
	// Check that both methods are present
	found1, found2 := false, false
	for _, method := range methods {
		if method == "method1" {
			found1 = true
		}
		if method == "method2" {
			found2 = true
		}
	}
	
	if !found1 || !found2 {
		t.Error("Not all registered methods were returned")
	}
}

func TestJSONRPCEngine_ProcessRequest_ValidRequest(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	// Register a test method
	engine.RegisterMethod("echo", func(params interface{}) (interface{}, error) {
		return params, nil
	})
	
	// Create a valid JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "echo",
		"params":  "hello world",
		"id":      1,
	}
	
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	
	responseData := engine.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	// Check response
	if response.JSONRPC != "2.0" {
		t.Errorf("Expected jsonrpc version 2.0, got %s", response.JSONRPC)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	if response.Result != "hello world" {
		t.Errorf("Expected result 'hello world', got %v", response.Result)
	}
	
	if response.ID != float64(1) { // JSON unmarshaling converts numbers to float64
		t.Errorf("Expected ID 1, got %v", response.ID)
	}
}

func TestJSONRPCEngine_ProcessRequest_InvalidJSON(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	invalidJSON := []byte(`{"jsonrpc": "2.0", "method": "test", invalid}`)
	responseData := engine.ProcessRequest(invalidJSON)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}
	
	if response.Error == nil {
		t.Error("Expected parse error, got nil")
	}
	
	if response.Error.Code != ErrorCodeParseError {
		t.Errorf("Expected parse error code %d, got %d", ErrorCodeParseError, response.Error.Code)
	}
}

func TestJSONRPCEngine_ProcessRequest_InvalidVersion(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	request := map[string]interface{}{
		"jsonrpc": "1.0",
		"method":  "test",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := engine.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}
	
	if response.Error == nil {
		t.Error("Expected invalid request error, got nil")
	}
	
	if response.Error.Code != ErrorCodeInvalidRequest {
		t.Errorf("Expected invalid request error code %d, got %d", ErrorCodeInvalidRequest, response.Error.Code)
	}
}

func TestJSONRPCEngine_ProcessRequest_MethodNotFound(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "nonexistent",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := engine.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}
	
	if response.Error == nil {
		t.Error("Expected method not found error, got nil")
	}
	
	if response.Error.Code != ErrorCodeMethodNotFound {
		t.Errorf("Expected method not found error code %d, got %d", ErrorCodeMethodNotFound, response.Error.Code)
	}
	
	if !strings.Contains(response.Error.Message, "Method not found") {
		t.Errorf("Error message should contain 'Method not found', got: %s", response.Error.Message)
	}
}

func TestJSONRPCEngine_ProcessRequest_MethodError(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	// Register a method that returns an error
	engine.RegisterMethod("error_method", func(params interface{}) (interface{}, error) {
		return nil, &testError{message: "test error"}
	})
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "error_method",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := engine.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}
	
	if response.Error == nil {
		t.Error("Expected internal error, got nil")
	}
	
	if response.Error.Code != ErrorCodeInternalError {
		t.Errorf("Expected internal error code %d, got %d", ErrorCodeInternalError, response.Error.Code)
	}
}

func TestJSONRPCEngine_ProcessRequest_WithComplexParams(t *testing.T) {
	engine := NewJSONRPCEngine()
	
	// Register a method that handles complex parameters
	engine.RegisterMethod("complex", func(params interface{}) (interface{}, error) {
		paramMap, ok := params.(map[string]interface{})
		if !ok {
			return nil, &testError{message: "params must be an object"}
		}
		
		name := paramMap["name"].(string)
		age := paramMap["age"].(float64)
		
		return map[string]interface{}{
			"greeting": "Hello " + name,
			"next_age": age + 1,
		}, nil
	})
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "complex",
		"params": map[string]interface{}{
			"name": "Alice",
			"age":  30,
		},
		"id": 1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := engine.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	if result["greeting"] != "Hello Alice" {
		t.Errorf("Expected greeting 'Hello Alice', got %v", result["greeting"])
	}
	
	if result["next_age"] != float64(31) {
		t.Errorf("Expected next_age 31, got %v", result["next_age"])
	}
}

// Test helper error type
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}