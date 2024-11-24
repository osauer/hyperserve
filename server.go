// Copyright 2024 by Oliver Sauer
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

// Simple HTTP Server with MiddlewareFunc and various option to handle requests and responses.

package hyperserve

import (
	"context"
	"crypto/tls"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// logger is a global logger for the httpServer. Use NewServer() to create a new httpServer with a custom logger.
var logger = slog.Default()

func init() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	logger.Info("Server initializing...")
}

// Environment management variable names
const (
	paramServerAddr = "SERVER_ADDR"
	paramHealthAddr = "HEALTH_ADDR"
	paramFileName   = "options.json"
)

var (
	isReady   atomic.Bool
	isRunning atomic.Bool

	// Server metrics
	totalRequests     atomic.Uint64
	totalResponseTime atomic.Int64
	serverStart       time.Time

	clientLimiters = sync.Map{}
)

// rateLimit limits requests per second that can be requested from the httpServer. Requires to add [RateLimitMiddleware]
type rateLimit = rate.Limit

// Server represents an HTTP httpServer that can handle requests and responses.
type Server struct {
	mux          *http.ServeMux
	healthMux    *http.ServeMux
	httpServer   *http.Server
	healthServer *http.Server
	middleware   *MiddlewareRegistry
	Options      *ServerOptions
}

// NewServer creates a new instance of the Server.
func NewServer(opts ...ServerOptionFunc) (*Server, error) {

	// init new httpServer
	srv := &Server{
		mux:        http.NewServeMux(),
		Options:    NewServerOptions(),
		middleware: NewMiddlewareRegistry(DefaultMiddleware()),
	}

	// apply httpServer options
	for _, opt := range opts {
		opt(srv)
	}

	isReady.Store(true)
	return srv, nil
}

// Run starts the httpServer and listens for incoming requests.
func (srv *Server) Run() error {
	// log httpServer start time for collection up-time metric
	serverStart = time.Now()
	isRunning.Store(true)

	// initialize the underlying http httpServer for serving requests
	srv.httpServer = &http.Server{
		Handler:      srv.mux,
		ReadTimeout:  srv.Options.ReadTimeout,
		WriteTimeout: srv.Options.WriteTimeout,
		IdleTimeout:  srv.Options.IdleTimeout,
	}
	srv.httpServer.RegisterOnShutdown(srv.logServerMetrics)

	// apply available middleware to the httpServer
	srv.httpServer.Handler = srv.middleware.applyToMux(srv.mux)

	if srv.Options.RunHealthServer {
		srv.initHealthServer()
	}

	// Channel for server errors
	serverErr := make(chan error, 1)

	// Run the server in a goroutine
	go func() {
		if srv.Options.EnableTLS {
			if srv.Options.CertFile == "" || srv.Options.KeyFile == "" {
				logger.Error("TLS enabled but no key or cert file provided.", "key", srv.Options.KeyFile,
					"cert", srv.Options.CertFile)
				return
			}
			// Configure TLS settings
			srv.httpServer.TLSConfig = srv.tlsConfig()
			srv.httpServer.Addr = srv.Options.TLSAddr
			logger.Info("Starting TLS server on", "addr", srv.Options.TLSAddr)
			serverErr <- srv.httpServer.ListenAndServeTLS(srv.Options.CertFile, srv.Options.KeyFile)
		} else {
			srv.httpServer.Addr = srv.Options.Addr
			logger.Info("Starting server on", "addr", srv.Options.Addr)
			serverErr <- srv.httpServer.ListenAndServe()
		}
	}()

	// Graceful shutdown handling
	return srv.handleShutdown(serverErr)
}

func (srv *Server) logServerMetrics() {
	tp := uint64(0)
	resp := totalResponseTime.Load()
	if resp != 0 {
		tp = totalRequests.Load() / uint64(resp)
	}
	upTime := time.Since(serverStart)
	logger.Info("Server metrics:", "up-time", upTime, "µs-in-handlers", resp, "total-req",
		totalRequests.Load(),
		"avg-handles-per-µs", tp)
}

func (srv *Server) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}

func (srv *Server) AddMiddleware(route string, mw MiddlewareFunc) {
	srv.middleware.Add(route, MiddlewareStack{mw})
}
func (srv *Server) AddMiddlewareStack(route string, mw MiddlewareStack) {
	srv.middleware.Add(route, mw)
}

// helper function to initialise the health server
func (srv *Server) initHealthServer() {
	// initialize a lightweight http  for health endpoints listening on different port
	srv.healthMux = http.NewServeMux()
	srv.healthMux.HandleFunc("/healthz/", srv.healthzHandler)
	srv.healthMux.HandleFunc("/readyz/", srv.readyzHandler)
	srv.healthMux.HandleFunc("/livez/", srv.livezHandler)

	// Todo add TLS check, make this more generic
	srv.healthServer = &http.Server{
		Addr:    srv.Options.HealthAddr,
		Handler: srv.healthMux,
		BaseContext: func(_ net.Listener) context.Context {
			return context.WithValue(context.Background(), "health", true)
		},
	}
	go func() {
		logger.Info("Starting health server.", "addr", srv.Options.HealthAddr)
		if err := srv.healthServer.ListenAndServe(); err != nil {
			logger.Error("Health server failed to start.", "error", err)
		}
	}()

}

func (srv *Server) handleShutdown(serverErr chan error) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	select {
	case sig := <-quit:
		logger.Info("Shutting down server.", "reason", sig)
		isReady.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		isRunning.Store(false)
		return srv.shutdown(ctx)
	case err := <-serverErr:
		return err
	}
}

func (srv *Server) shutdown(ctx context.Context) error {
	ret := error(nil)
	if err := srv.shutdownHealthServer(ctx); err != nil {
		logger.Error("Error during health server shutdown.", "error", err)
		ret = err
	}
	if err := srv.httpServer.Shutdown(ctx); err != nil {
		logger.Error("Error during server shutdown.", "error", err)
		ret = err
	}
	return ret
}

func (srv *Server) shutdownHealthServer(ctx context.Context) error {
	if srv.Options.RunHealthServer {
		logger.Info("Shutting down health server.")
		// Close any dependencies if needed
		// ...
		return srv.healthServer.Shutdown(ctx)
	} else {
		return nil
	}
}

/*func (srv *Server) With(middleware ...MiddlewareFunc) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after httpServer has started.")
	}
	srv.middleware = append(srv.middleware, middleware...)
	return srv
}

func (srv *Server) WithOut(middleware ...MiddlewareFunc) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after httpServer has started.")
	}
	srv.exclude = append(srv.exclude, middleware...)
	return srv
}*/
/*
func (srv *Server) WithStack(stack MiddlewareStack) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after httpServer has started.")
	}
	srv.middleware = append(srv.middleware, stack...)
	return srv
}*/

func (srv *Server) WithOutStack(stack MiddlewareStack) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after httpServer has started.")
	}
	srv.middleware.exclude = append(srv.middleware.exclude, stack...)
	return srv
}

// Handle registers the handler function for the given pattern.
// Example usage:
//
//	srv.Handle("/static", http.FileServer(http.Dir("./static")))
func (srv *Server) Handle(pattern string, handlerFunc http.HandlerFunc) {
	srv.mux.Handle(pattern, handlerFunc)
}

// HandleFunc registers the handler function for the given pattern.
// Example usage:
//
//	srv.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
//	    fmt.Fprintln(w, "Hello, world!")
//	})
func (srv *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	srv.mux.HandleFunc(pattern, handler)
}

// HandleFuncDynamic registers a handler function for the given pattern that dynamically generates data for the template.
// Example usage:
//
//	srv.HandleFuncDynamic("/time", "time.html", func(r *http.Request) interface{} {
//	    return map[string]interface{}{
//	        "timestamp": time.Now().Format("2006-01-02 15:04:05"),
//	    }
//	})
func (srv *Server) HandleFuncDynamic(pattern, template string, dataFunc DataFunc) {
	srv.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		data := dataFunc(r)
		err := templates.ExecuteTemplate(w, template, data)
		if err != nil {
			http.Error(w, "Failed to load content", http.StatusInternalServerError)
			logger.Error("Failed to load content", "error", err)
			return
		}
	})
}

// Cache templates for efficiency, with lazy initialisation
var templates *template.Template = nil

func EnsureTrailingSlash(dir string) string {
	if dir != "" && !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return dir
}

func (srv *Server) HandleStatic(pattern string) {
	staticDir := EnsureTrailingSlash(srv.Options.StaticDir)
	srv.mux.Handle(pattern, http.StripPrefix(pattern, http.FileServer(http.Dir(staticDir))))
}

func (srv *Server) HandleTemplate(pattern, t string, data interface{}) *Server {
	templateDir := EnsureTrailingSlash(srv.Options.TemplateDir)
	if templates == nil {
		// lazy initialisation of templates
		parseTemplates(templateDir, srv)
	}
	srv.mux.HandleFunc(pattern, templateHandler(t, data))
	return srv
}

func parseTemplates(templateDir string, srv *Server) {
	// check if an absolute path is provided and provide a warning
	if filepath.IsAbs(templateDir) {
		logger.Warn("Absolute path provided for template directory", "dir", templateDir)
	}

	// check if the template directory exists to avoid panic
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		wd, _ := os.Getwd()
		ad, _ := filepath.Abs(templateDir)
		logger.Error("Template directory not found", "error", err, "working-dir", wd, "abs-path", ad)
		srv.logServerMetrics()
		os.Exit(1)
	}
	templates = template.Must(template.ParseGlob(templateDir + "*.html"))
	logger.Info("Templates parsed.", "pattern", templateDir+"*.html")
}

// DataFunc returns a data structure for the template.
type DataFunc func(r *http.Request) interface{}

// WithTLS enables TLS on the httpServer.
func WithTLS(certFile, keyFile string) ServerOptionFunc {
	return func(srv *Server) {
		srv.Options.EnableTLS = true
		wd, _ := os.Getwd()
		// do not override existing values if not set
		if certFile != "" {
			srv.Options.CertFile = certFile
		}
		if keyFile != "" {
			srv.Options.KeyFile = keyFile
		}
		// check if the key and cert files exist
		_, err := os.Stat(certFile)
		if err != nil {
			logger.Error("Cert file not found", "error", err, "file", certFile, "wd", wd)
			os.Exit(1)
		}
		_, err = os.Stat(keyFile)
		if err != nil {
			logger.Error("Key file not found", "error", err, "file", keyFile, "wd", wd)
			os.Exit(1)
		}
	}
}

// WithHealthServer enables the health server on a different port.
func WithLoglevel(level slog.Level) ServerOptionFunc {
	return func(srv *Server) {
		slog.SetLogLoggerLevel(level)
	}
}

// WithHealthServer enables the health server on a different port.
func WithHealthServer() ServerOptionFunc {
	return func(srv *Server) {
		srv.Options.RunHealthServer = true
	}
}

// WithAddr is a configuration option for the server to define listener port
func WithAddr(addr string) ServerOptionFunc {
	return func(srv *Server) {
		// validate the address
		_, port, err := net.SplitHostPort(addr)
		if err != nil && port == "" {
			logger.Error("setting address option", "error", err)
			// if the address failed to set, we must exit (no fallback to default etc.)
			os.Exit(1)
		}
		srv.Options.Addr = addr
	}
}

// WithLogger replaces the default with a custom logger.
func WithLogger(l *slog.Logger) ServerOptionFunc {
	return func(srv *Server) {
		logger = l
	}
}

// WithTimeouts adds timeouts to the httpServer.
func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) ServerOptionFunc {
	return func(srv *Server) {
		srv.setTimeouts(readTimeout, writeTimeout, idleTimeout)
	}
}

// WithRateLimit sets rate limiting parameters of the httpServer.
func WithRateLimit(limit rateLimit, burst int) ServerOptionFunc {
	return func(srv *Server) {
		srv.Options.RateLimit = limit
		srv.Options.Burst = burst
	}
}

// WithTemplateDir sets the directory for the templates.
func WithTemplateDir(dir string) ServerOptionFunc {
	return func(srv *Server) {
		srv.Options.TemplateDir = dir
	}
}