package hyperserve

import (
	"log/slog"

	pkgjsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
	pkgserver "github.com/osauer/hyperserve/pkg/server"
)

const (
	// JSONRPCVersion is re-exported from pkg/jsonrpc for backwards compatibility.
	JSONRPCVersion = pkgjsonrpc.Version
)

type (
	// JSONRPCRequest re-exports pkg/jsonrpc.Request.
	JSONRPCRequest = pkgjsonrpc.Request
	// JSONRPCResponse re-exports pkg/jsonrpc.Response.
	JSONRPCResponse = pkgjsonrpc.Response
	// JSONRPCError re-exports pkg/jsonrpc.ErrorDetails.
	JSONRPCError = pkgjsonrpc.ErrorDetails
	// JSONRPCMethodHandler re-exports pkg/jsonrpc.MethodHandler.
	JSONRPCMethodHandler = pkgjsonrpc.MethodHandler
	// JSONRPCEngine re-exports pkg/jsonrpc.Engine.
	JSONRPCEngine = pkgjsonrpc.Engine
)

const (
	ErrorCodeParseError     = pkgjsonrpc.ErrorCodeParseError
	ErrorCodeInvalidRequest = pkgjsonrpc.ErrorCodeInvalidRequest
	ErrorCodeMethodNotFound = pkgjsonrpc.ErrorCodeMethodNotFound
	ErrorCodeInvalidParams  = pkgjsonrpc.ErrorCodeInvalidParams
	ErrorCodeInternalError  = pkgjsonrpc.ErrorCodeInternalError
)

// NewJSONRPCEngine preserves the public API while delegating to pkg/jsonrpc.
func NewJSONRPCEngine() *JSONRPCEngine {
	return pkgjsonrpc.NewEngine(pkgserver.DefaultLogger())
}

// NewJSONRPCEngineWithLogger allows callers to override the logger used by the engine.
func NewJSONRPCEngineWithLogger(log *slog.Logger) *JSONRPCEngine {
	return pkgjsonrpc.NewEngine(log)
}
