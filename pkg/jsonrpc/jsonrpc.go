package jsonrpc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
)

// Version is the JSON-RPC 2.0 version identifier.
const Version = "2.0"

// Request represents a JSON-RPC 2.0 request message.
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

// Response represents a JSON-RPC 2.0 response message.
type Response struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *ErrorDetails `json:"error,omitempty"`
	ID      interface{}   `json:"id"`
}

// ErrorDetails represents a JSON-RPC 2.0 error object.
type ErrorDetails struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

// MethodHandler defines the signature for JSON-RPC method handlers.
type MethodHandler func(params interface{}) (interface{}, error)

// Engine handles JSON-RPC 2.0 request processing.
type Engine struct {
	methods map[string]MethodHandler
	logger  *slog.Logger
}

// NewEngine creates a new JSON-RPC engine.
func NewEngine(logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		methods: make(map[string]MethodHandler),
		logger:  logger,
	}
}

// RegisterMethod registers a method handler with the JSON-RPC engine.
func (engine *Engine) RegisterMethod(name string, handler MethodHandler) {
	engine.methods[name] = handler
	engine.logger.Debug("JSON-RPC method registered", "method", name)
}

// ProcessRequest processes a JSON-RPC request payload and returns a response payload.
func (engine *Engine) ProcessRequest(requestData []byte) []byte {
	var request Request

	// Parse the request
	if err := json.Unmarshal(requestData, &request); err != nil {
		engine.logger.Error("Failed to parse JSON-RPC request", "error", err)
		response := &Response{
			JSONRPC: Version,
			Error: &ErrorDetails{
				Code:    ErrorCodeParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
			ID: nil,
		}
		responseData, _ := json.Marshal(response)
		return responseData
	}

	response := engine.ProcessRequestDirect(&request)
	responseData, err := json.Marshal(response)
	if err != nil {
		engine.logger.Error("Failed to marshal JSON-RPC response", "error", err)
		errorResponse := &Response{
			JSONRPC: Version,
			Error: &ErrorDetails{
				Code:    ErrorCodeInternalError,
				Message: "Internal error",
				Data:    "failed to marshal response",
			},
			ID: response.ID,
		}
		errorResponseData, _ := json.Marshal(errorResponse)
		return errorResponseData
	}

	engine.logger.Debug("JSON-RPC request processed successfully", "method", request.Method)
	return responseData
}

// ProcessRequestDirect processes a JSON-RPC request object and returns the response object.
func (engine *Engine) ProcessRequestDirect(request *Request) *Response {
	// Validate JSON-RPC version
	if request.JSONRPC != Version {
		engine.logger.Error("Invalid JSON-RPC version", "version", request.JSONRPC)
		return &Response{
			JSONRPC: Version,
			Error: &ErrorDetails{
				Code:    ErrorCodeInvalidRequest,
				Message: "Invalid Request",
				Data:    fmt.Sprintf("expected jsonrpc version %s", Version),
			},
			ID: request.ID,
		}
	}

	// Find method handler
	handler, exists := engine.methods[request.Method]
	if !exists {
		engine.logger.Error("JSON-RPC method not found", "method", request.Method)
		return &Response{
			JSONRPC: Version,
			Error: &ErrorDetails{
				Code:    ErrorCodeMethodNotFound,
				Message: "Method not found",
				Data:    fmt.Sprintf("method '%s' not found", request.Method),
			},
			ID: request.ID,
		}
	}

	// Call method handler
	result, err := handler(request.Params)
	if err != nil {
		engine.logger.Error("JSON-RPC method execution error", "method", request.Method, "error", err)
		return &Response{
			JSONRPC: Version,
			Error: &ErrorDetails{
				Code:    ErrorCodeInternalError,
				Message: "Internal error",
				Data:    err.Error(),
			},
			ID: request.ID,
		}
	}

	return &Response{
		JSONRPC: Version,
		Result:  result,
		ID:      request.ID,
	}
}

// GetRegisteredMethods returns a sorted list of registered method names.
func (engine *Engine) GetRegisteredMethods() []string {
	methods := make([]string, 0, len(engine.methods))
	for name := range engine.methods {
		methods = append(methods, name)
	}
	sort.Strings(methods)
	return methods
}
