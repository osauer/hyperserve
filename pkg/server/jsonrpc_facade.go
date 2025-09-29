package server

import pkgjsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"

type (
	JSONRPCRequest       = pkgjsonrpc.Request
	JSONRPCResponse      = pkgjsonrpc.Response
	JSONRPCError         = pkgjsonrpc.ErrorDetails
	JSONRPCEngine        = pkgjsonrpc.Engine
	JSONRPCMethodHandler = pkgjsonrpc.MethodHandler
)

const (
	JSONRPCVersion          = pkgjsonrpc.Version
	ErrorCodeParseError     = pkgjsonrpc.ErrorCodeParseError
	ErrorCodeInvalidRequest = pkgjsonrpc.ErrorCodeInvalidRequest
	ErrorCodeMethodNotFound = pkgjsonrpc.ErrorCodeMethodNotFound
	ErrorCodeInvalidParams  = pkgjsonrpc.ErrorCodeInvalidParams
	ErrorCodeInternalError  = pkgjsonrpc.ErrorCodeInternalError
)

func NewJSONRPCEngine() *JSONRPCEngine {
	return pkgjsonrpc.NewEngine(logger)
}
