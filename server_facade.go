package hyperserve

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	pkgserver "github.com/osauer/hyperserve/pkg/server"
)

type (
	Server                = pkgserver.Server
	ServerOptions         = pkgserver.ServerOptions
	ServerOptionFunc      = pkgserver.ServerOptionFunc
	MiddlewareFunc        = pkgserver.MiddlewareFunc
	MiddlewareStack       = pkgserver.MiddlewareStack
	MiddlewareRegistry    = pkgserver.MiddlewareRegistry
	RateLimit             = pkgserver.RateLimit
	InterceptorChain      = pkgserver.InterceptorChain
	Interceptor           = pkgserver.Interceptor
	InterceptableRequest  = pkgserver.InterceptableRequest
	InterceptableResponse = pkgserver.InterceptableResponse
	InterceptorResponse   = pkgserver.InterceptorResponse
	AuthTokenInjector     = pkgserver.AuthTokenInjector
	RequestLogger         = pkgserver.RequestLogger
	ResponseTransformer   = pkgserver.ResponseTransformer
	RateLimitInterceptor  = pkgserver.RateLimitInterceptor
	RateLimiter           = pkgserver.RateLimiter
	Upgrader              = pkgserver.Upgrader
	Conn                  = pkgserver.Conn
	CORSOptions           = pkgserver.CORSOptions
	SSEMessage            = pkgserver.SSEMessage
)

const (
	GlobalMiddlewareRoute        = pkgserver.GlobalMiddlewareRoute
	TextMessage                  = pkgserver.TextMessage
	BinaryMessage                = pkgserver.BinaryMessage
	CloseMessage                 = pkgserver.CloseMessage
	PingMessage                  = pkgserver.PingMessage
	PongMessage                  = pkgserver.PongMessage
	CloseNormalClosure           = pkgserver.CloseNormalClosure
	CloseGoingAway               = pkgserver.CloseGoingAway
	CloseProtocolError           = pkgserver.CloseProtocolError
	CloseUnsupportedData         = pkgserver.CloseUnsupportedData
	CloseNoStatusReceived        = pkgserver.CloseNoStatusReceived
	CloseAbnormalClosure         = pkgserver.CloseAbnormalClosure
	CloseInvalidFramePayloadData = pkgserver.CloseInvalidFramePayloadData
	ClosePolicyViolation         = pkgserver.ClosePolicyViolation
	CloseMessageTooBig           = pkgserver.CloseMessageTooBig
	CloseMandatoryExtension      = pkgserver.CloseMandatoryExtension
	CloseInternalServerError     = pkgserver.CloseInternalServerError
	CloseServiceRestart          = pkgserver.CloseServiceRestart
	CloseTryAgainLater           = pkgserver.CloseTryAgainLater
	CloseTLSHandshake            = pkgserver.CloseTLSHandshake
	MCPVersion                   = pkgserver.MCPVersion
	HTTPTransport                = pkgserver.HTTPTransport
	StdioTransport               = pkgserver.StdioTransport
	LevelDebug                   = pkgserver.LevelDebug
	LevelInfo                    = pkgserver.LevelInfo
	LevelWarn                    = pkgserver.LevelWarn
	LevelError                   = pkgserver.LevelError
)

var (
	ErrNotWebSocket = pkgserver.ErrNotWebSocket
	ErrBadHandshake = pkgserver.ErrBadHandshake
)

func NewServer(opts ...ServerOptionFunc) (*Server, error) {
	return pkgserver.NewServer(opts...)
}

func NewServerOptions() *ServerOptions { return pkgserver.NewServerOptions() }

func EnsureTrailingSlash(dir string) string { return pkgserver.EnsureTrailingSlash(dir) }

func GetVersionInfo() string { return pkgserver.GetVersionInfo() }

func WithTLS(certFile, keyFile string) ServerOptionFunc { return pkgserver.WithTLS(certFile, keyFile) }

func WithLoglevel(level slog.Level) ServerOptionFunc { return pkgserver.WithLoglevel(level) }

func WithDebugMode() ServerOptionFunc { return pkgserver.WithDebugMode() }

func WithSuppressBanner(suppress bool) ServerOptionFunc {
	return pkgserver.WithSuppressBanner(suppress)
}

func WithOnShutdown(hook func(context.Context) error) ServerOptionFunc {
	return pkgserver.WithOnShutdown(hook)
}

func WithHealthServer() ServerOptionFunc { return pkgserver.WithHealthServer() }

func WithAddr(addr string) ServerOptionFunc { return pkgserver.WithAddr(addr) }

func WithLogger(l *slog.Logger) ServerOptionFunc { return pkgserver.WithLogger(l) }

func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) ServerOptionFunc {
	return pkgserver.WithTimeouts(readTimeout, writeTimeout, idleTimeout)
}

func WithReadTimeout(timeout time.Duration) ServerOptionFunc {
	return pkgserver.WithReadTimeout(timeout)
}

func WithWriteTimeout(timeout time.Duration) ServerOptionFunc {
	return pkgserver.WithWriteTimeout(timeout)
}

func WithIdleTimeout(timeout time.Duration) ServerOptionFunc {
	return pkgserver.WithIdleTimeout(timeout)
}

func WithReadHeaderTimeout(timeout time.Duration) ServerOptionFunc {
	return pkgserver.WithReadHeaderTimeout(timeout)
}

func WithRateLimit(limit RateLimit, burst int) ServerOptionFunc {
	return pkgserver.WithRateLimit(limit, burst)
}

func WithCORS(opts *CORSOptions) ServerOptionFunc { return pkgserver.WithCORS(opts) }

func WithTemplateDir(dir string) ServerOptionFunc { return pkgserver.WithTemplateDir(dir) }

func WithAuthTokenValidator(validator func(string) (bool, error)) ServerOptionFunc {
	return pkgserver.WithAuthTokenValidator(validator)
}

func WithFIPSMode() ServerOptionFunc { return pkgserver.WithFIPSMode() }

func WithHardenedMode() ServerOptionFunc { return pkgserver.WithHardenedMode() }

func WithEncryptedClientHello(echKeys ...[]byte) ServerOptionFunc {
	return pkgserver.WithEncryptedClientHello(echKeys...)
}

func WithMCPSupport(name, version string, configs ...MCPTransportConfig) ServerOptionFunc {
	return pkgserver.WithMCPSupport(name, version, configs...)
}

func WithMCPNamespace(name string, configs ...MCPNamespaceConfig) ServerOptionFunc {
	return pkgserver.WithMCPNamespace(name, configs...)
}

func WithMCPEndpoint(endpoint string) ServerOptionFunc { return pkgserver.WithMCPEndpoint(endpoint) }

func WithMCPServerInfo(name, version string) ServerOptionFunc {
	return pkgserver.WithMCPServerInfo(name, version)
}

func WithMCPFileToolRoot(rootDir string) ServerOptionFunc {
	return pkgserver.WithMCPFileToolRoot(rootDir)
}

func WithMCPToolsDisabled() ServerOptionFunc { return pkgserver.WithMCPToolsDisabled() }

func WithMCPBuiltinTools(enabled bool) ServerOptionFunc {
	return pkgserver.WithMCPBuiltinTools(enabled)
}

func WithMCPBuiltinResources(enabled bool) ServerOptionFunc {
	return pkgserver.WithMCPBuiltinResources(enabled)
}

func WithMCPResourcesDisabled() ServerOptionFunc { return pkgserver.WithMCPResourcesDisabled() }

func WithCSPWebWorkerSupport() ServerOptionFunc { return pkgserver.WithCSPWebWorkerSupport() }

func WithMCPDiscoveryPolicy(policy pkgserver.DiscoveryPolicy) ServerOptionFunc {
	return pkgserver.WithMCPDiscoveryPolicy(policy)
}

func WithMCPDiscoveryFilter(filter func(string, *http.Request) bool) ServerOptionFunc {
	return pkgserver.WithMCPDiscoveryFilter(filter)
}

func DefaultMiddleware(srv *Server) MiddlewareStack { return pkgserver.DefaultMiddleware(srv) }

func SecureAPI(srv *Server) MiddlewareStack { return pkgserver.SecureAPI(srv) }

func SecureWeb(options *ServerOptions) MiddlewareStack { return pkgserver.SecureWeb(options) }

func FileServer(options *ServerOptions) MiddlewareStack { return pkgserver.FileServer(options) }

func MetricsMiddleware(srv *Server) MiddlewareFunc { return pkgserver.MetricsMiddleware(srv) }

func AuthMiddleware(options *ServerOptions) MiddlewareFunc { return pkgserver.AuthMiddleware(options) }

func RequestLoggerMiddleware(next http.Handler) http.HandlerFunc {
	return pkgserver.RequestLoggerMiddleware(next)
}

func ResponseTimeMiddleware(next http.Handler) http.HandlerFunc {
	return pkgserver.ResponseTimeMiddleware(next)
}

func RecoveryMiddleware(next http.Handler) http.HandlerFunc {
	return pkgserver.RecoveryMiddleware(next)
}

func RateLimitMiddleware(srv *Server) MiddlewareFunc { return pkgserver.RateLimitMiddleware(srv) }

func HeadersMiddleware(options *ServerOptions) MiddlewareFunc {
	return pkgserver.HeadersMiddleware(options)
}

func ChaosMiddleware(options *ServerOptions) MiddlewareFunc {
	return pkgserver.ChaosMiddleware(options)
}

func TraceMiddleware(next http.Handler) http.HandlerFunc { return pkgserver.TraceMiddleware(next) }

func NewMiddlewareRegistry(globalMiddleware MiddlewareStack) *MiddlewareRegistry {
	return pkgserver.NewMiddlewareRegistry(globalMiddleware)
}

func DefaultLogger() *slog.Logger { return pkgserver.DefaultLogger() }

func SetDefaultLogger(l *slog.Logger) { pkgserver.SetDefaultLogger(l) }

func WebSocketUpgrader(srv *Server) *Upgrader { return srv.WebSocketUpgrader() }

func DefaultCheckOrigin(r *http.Request) bool { return pkgserver.DefaultCheckOrigin(r) }

func CheckOriginWithAllowedList(origins []string) func(*http.Request) bool {
	return pkgserver.CheckOriginWithAllowedList(origins)
}

func HealthCheckHandler(w http.ResponseWriter, r *http.Request) { pkgserver.HealthCheckHandler(w, r) }

func NewSSEMessage(data any) *SSEMessage { return pkgserver.NewSSEMessage(data) }

func PanicHandler(w http.ResponseWriter, r *http.Request) { pkgserver.PanicHandler(w, r) }

func NewInterceptorChain() *InterceptorChain { return pkgserver.NewInterceptorChain() }

func NewAuthTokenInjector(provider func(context.Context) (string, error)) *AuthTokenInjector {
	return pkgserver.NewAuthTokenInjector(provider)
}

func NewRequestLogger(logger func(format string, args ...interface{})) *RequestLogger {
	return pkgserver.NewRequestLogger(logger)
}

func NewResponseTransformer(transformer func([]byte, string) ([]byte, error)) *ResponseTransformer {
	return pkgserver.NewResponseTransformer(transformer)
}

func NewRateLimitInterceptor(limiter RateLimiter) *RateLimitInterceptor {
	return pkgserver.NewRateLimitInterceptor(limiter)
}

func IsCloseError(err error, codes ...int) bool { return pkgserver.IsCloseError(err, codes...) }

func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	return pkgserver.IsUnexpectedCloseError(err, expectedCodes...)
}
