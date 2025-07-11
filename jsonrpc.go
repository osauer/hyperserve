package hyperserve

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// JSONRPCVersion is the JSON-RPC 2.0 version identifier
const JSONRPCVersion = "2.0"

// JSONRPCRequest represents a JSON-RPC 2.0 request message
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response message
type JSONRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	Result  interface{}    `json:"result,omitempty"`
	Error   *JSONRPCError  `json:"error,omitempty"`
	ID      interface{}    `json:"id"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

// JSONRPCMethodHandler defines the signature for JSON-RPC method handlers
type JSONRPCMethodHandler func(params interface{}) (interface{}, error)

// JSONRPCEngine handles JSON-RPC 2.0 request processing
type JSONRPCEngine struct {
	methods map[string]JSONRPCMethodHandler
	logger  *slog.Logger
}

// NewJSONRPCEngine creates a new JSON-RPC engine
func NewJSONRPCEngine() *JSONRPCEngine {
	return &JSONRPCEngine{
		methods: make(map[string]JSONRPCMethodHandler),
		logger:  logger,
	}
}

// RegisterMethod registers a method handler with the JSON-RPC engine
func (engine *JSONRPCEngine) RegisterMethod(name string, handler JSONRPCMethodHandler) {
	engine.methods[name] = handler
	engine.logger.Debug("JSON-RPC method registered", "method", name)
}

// ProcessRequest processes a JSON-RPC request and returns a response
func (engine *JSONRPCEngine) ProcessRequest(requestData []byte) []byte {
	var request JSONRPCRequest
	
	// Parse the request
	if err := json.Unmarshal(requestData, &request); err != nil {
		engine.logger.Error("Failed to parse JSON-RPC request", "error", err)
		response := &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
			ID: nil,
		}
		responseData, _ := json.Marshal(response)
		return responseData
	}
	
	// Validate JSON-RPC version
	if request.JSONRPC != JSONRPCVersion {
		engine.logger.Error("Invalid JSON-RPC version", "version", request.JSONRPC)
		response := &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeInvalidRequest,
				Message: "Invalid Request",
				Data:    fmt.Sprintf("expected jsonrpc version %s", JSONRPCVersion),
			},
			ID: request.ID,
		}
		responseData, _ := json.Marshal(response)
		return responseData
	}
	
	// Find method handler
	handler, exists := engine.methods[request.Method]
	if !exists {
		engine.logger.Error("JSON-RPC method not found", "method", request.Method)
		response := &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeMethodNotFound,
				Message: "Method not found",
				Data:    fmt.Sprintf("method '%s' not found", request.Method),
			},
			ID: request.ID,
		}
		responseData, _ := json.Marshal(response)
		return responseData
	}
	
	// Call method handler
	result, err := handler(request.Params)
	if err != nil {
		engine.logger.Error("JSON-RPC method execution error", "method", request.Method, "error", err)
		response := &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeInternalError,
				Message: "Internal error",
				Data:    err.Error(),
			},
			ID: request.ID,
		}
		responseData, _ := json.Marshal(response)
		return responseData
	}
	
	// Success response
	response := &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  result,
		ID:      request.ID,
	}
	
	responseData, err := json.Marshal(response)
	if err != nil {
		engine.logger.Error("Failed to marshal JSON-RPC response", "error", err)
		errorResponse := &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeInternalError,
				Message: "Internal error",
				Data:    "failed to marshal response",
			},
			ID: request.ID,
		}
		errorResponseData, _ := json.Marshal(errorResponse)
		return errorResponseData
	}
	
	engine.logger.Debug("JSON-RPC request processed successfully", "method", request.Method)
	return responseData
}

// ProcessRequestDirect processes a JSON-RPC request object directly and returns a response object
func (engine *JSONRPCEngine) ProcessRequestDirect(request *JSONRPCRequest) *JSONRPCResponse {
	// Validate request
	if request.JSONRPC != JSONRPCVersion {
		engine.logger.Error("Invalid JSON-RPC version", "version", request.JSONRPC)
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeInvalidRequest,
				Message: "Invalid Request",
				Data:    fmt.Sprintf("expected jsonrpc version %s", JSONRPCVersion),
			},
			ID: request.ID,
		}
	}
	
	// Find method handler
	handler, exists := engine.methods[request.Method]
	if !exists {
		engine.logger.Error("JSON-RPC method not found", "method", request.Method)
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
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
		return &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			Error: &JSONRPCError{
				Code:    ErrorCodeInternalError,
				Message: "Internal error",
				Data:    err.Error(),
			},
			ID: request.ID,
		}
	}
	
	// Build response
	response := &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  result,
		ID:      request.ID,
	}
	
	engine.logger.Debug("JSON-RPC request processed successfully", "method", request.Method)
	return response
}

// GetRegisteredMethods returns a list of all registered method names
func (engine *JSONRPCEngine) GetRegisteredMethods() []string {
	methods := make([]string, 0, len(engine.methods))
	for method := range engine.methods {
		methods = append(methods, method)
	}
	return methods
}