package mcp

import "fmt"

type toolError struct {
	message string
}

func (e *toolError) Error() string { return e.message }

// ToolError returns an error that tools can use for domain-level failures.
// The MCP handler converts it into a successful tools/call response with
// isError=true instead of a JSON-RPC protocol error.
func ToolError(message string) error {
	return &toolError{message: message}
}

// ToolErrorf formats a domain-level tool failure.
func ToolErrorf(format string, args ...any) error {
	return ToolError(fmt.Sprintf(format, args...))
}
