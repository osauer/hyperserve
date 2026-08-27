package hyperserve

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

const mcpDiscoveryEndpoint = "/.well-known/mcp.json"

func validateMCPEndpoint(endpoint string) error {
	if err := validateLiteralURLPath(endpoint, false, false); err != nil {
		return errors.New("MCP endpoint must be a clean, non-root literal URL path without a trailing slash, query, fragment, wildcard, or escape")
	}
	if endpoint == mcpDiscoveryEndpoint {
		return errors.New("MCP endpoint conflicts with the reserved discovery endpoint /.well-known/mcp.json")
	}
	return nil
}

func validateMiddlewarePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if err := validateLiteralURLPath(prefix, true, true); err != nil {
		return errors.New("middleware prefix must be empty or a clean literal URL path without a query, fragment, wildcard, or escape")
	}
	return nil
}

func validateLiteralURLPath(value string, allowRoot, allowTrailingSlash bool) error {
	if value == "" || !strings.HasPrefix(value, "/") {
		return errors.New("path must begin with '/'")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(value, "?#{}") {
		return errors.New("path is not a literal URL path")
	}
	// Escaped spellings can register one ServeMux pattern while requests expose
	// another decoded URL.Path. Reject them instead of making security scopes
	// depend on equivalent-but-different path text.
	if parsed.RawPath != "" || parsed.Path != value {
		return errors.New("escaped path is not allowed")
	}
	if !allowRoot && value == "/" {
		return errors.New("root path is not allowed")
	}
	if !allowTrailingSlash && value != "/" && strings.HasSuffix(value, "/") {
		return errors.New("trailing slash is not allowed")
	}

	canonical := path.Clean(value)
	if allowTrailingSlash && value != "/" && strings.HasSuffix(value, "/") {
		canonical += "/"
	}
	if canonical != value {
		return errors.New("path is not clean")
	}
	return nil
}
