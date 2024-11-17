// Copyright 2024 by Oliver Sauer
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

// Simple HTTP Server with MiddlewareFunc and various option to handle requests and responses.

package hyperserve

import (
	"context"
	"crypto/tls"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// logger is a global logger for the appServer. Use NewServer() to create a new appServer with a custom logger.
var logger = slog.Default()

func init() {
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

// rateLimit limits requests per second that can be requested from the appServer. Requires to add [RateLimitMiddleware]
type rateLimit = rate.Limit

func tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
}

// Server represents an HTTP appServer that can handle requests and responses.
type Server struct {
	mux               *http.ServeMux
	healthMux         *http.ServeMux
	appServer         *http.Server
	healthServer      *http.Server
	middleware        []MiddlewareFunc
	excludeMiddleware []MiddlewareFunc
	Options           *ServerOptions
}

// NewServer creates a new instance of the Server.
func NewServer(opts ...ServerOptionFunc) (*Server, error) {

	// init new appServer
	srv := &Server{
		mux:     http.NewServeMux(),
		Options: NewServerOptions(),
	}

	// make sure default middlewares are added
	srv.WithStack(DefaultMiddleware())

	// apply appServer options
	for _, opt := range opts {
		opt(srv)
	}

	// initialize the underlying http appServer for serving requests
	srv.appServer = &http.Server{
		Handler:      srv.mux,
		ReadTimeout:  srv.Options.ReadTimeout,
		WriteTimeout: srv.Options.WriteTimeout,
		IdleTimeout:  srv.Options.IdleTimeout,
	}
	srv.appServer.RegisterOnShutdown(srv.Shutdown)

	return srv, nil
}

// Run starts the appServer and listens for incoming requests.
func (srv *Server) Run() {
	// log appServer start time for collection up-time metric
	serverStart = time.Now()
	isReady.Store(true)
	isRunning.Store(true)

	srv.appServer.Handler = srv.chainMiddleware(srv.mux)

	if srv.Options.RunHealthServer {
		srv.initHealthServer()
	}

	go func() {
		if srv.Options.EnableTLS {
			if srv.Options.CertFile == "" || srv.Options.KeyFile == "" {
				logger.Error("TLS enabled but no key or cert file provided.", "key",
					srv.Options.KeyFile, "cert", srv.Options.CertFile)
				os.Exit(1)
			}
			srv.appServer.TLSConfig = tlsConfig()
			srv.appServer.Addr = srv.Options.TLSAddr

			logger.Info("Starting TLS server on", "addr", srv.Options.TLSAddr)
			if err := srv.appServer.ListenAndServeTLS(srv.Options.CertFile, srv.Options.KeyFile); err != nil && !errors.Is(
				err,
				http.ErrServerClosed) {
				logger.Error("Failed to start server.", "error", err)
				os.Exit(1)
			}
		} else {
			srv.appServer.Addr = srv.Options.Addr
			logger.Info("Starting server on", "addr", srv.Options.Addr)
			if err := srv.appServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("Failed to start server", "error", err)
				os.Exit(1)
			}

		}

	}()
	gracefulShutdown(srv)
}

// chainMiddleware helper to  apply multiple middlewares to a handler
func (s *Server) chainMiddleware(finalHandler http.Handler) http.Handler {
	handler := finalHandler
	mw := s.filteredMiddleware(s.middleware, s.excludeMiddleware)
	// reverse order to run first MiddlewareFunc passed first
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}

// filter middleware based on include and exclude stacks
func (s *Server) filteredMiddleware(include, exclude MiddlewareStack) MiddlewareStack {
	// compile the final middleware stack
	filtered := MiddlewareStack{}
	for _, mw := range s.middleware {
		if !slices.ContainsFunc(s.excludeMiddleware, func(excluded MiddlewareFunc) bool { return &mw == &excluded }) {
			filtered = append(filtered, mw)
		}
	}
	return filtered
}

func (srv *Server) Shutdown() {
	tp := uint64(0)
	resp := totalResponseTime.Load()
	if resp != 0 {
		tp = totalRequests.Load() / uint64(resp)
	}
	upTime := time.Since(serverStart)
	logger.Info("Server is shut down.", "up-time", upTime, "µs-in-handlers", resp, "total-req",
		totalRequests.Load(),
		"avg-handles-per-µs", tp)
}

// helper function to initialise the health server
func (srv *Server) initHealthServer() {
	// initialize a lightweight http  for health endpoints listening on different port
	srv.healthMux = http.NewServeMux()
	// Todo add TLS check, make this more generic
	srv.healthServer = &http.Server{
		Addr:    srv.Options.HealthAddr,
		Handler: srv.healthMux,
	}
	logger.Info("Health server initialised.", "addr", srv.Options.HealthAddr)

	// add built-in probing endpoints
	srv.healthMux.HandleFunc("/healthz/", srv.healthzHandler)
	srv.healthMux.HandleFunc("/readyz/", srv.readyzHandler)
	srv.healthMux.HandleFunc("/livez/", srv.livezHandler)
}

func gracefulShutdown(srv *Server) {

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)

	// block until a quit signal is received
	sig := <-quit
	logger.Info("Shutting down server...", "reason", sig)
	isReady.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.appServer.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown.", "error", err)
	}

	isRunning.Store(false)
}

// stop implements a graceful shutdown. Todo what's the diff between stop and shutdown? and then we have gracefulShutdown
func (srv *Server) stop(done chan struct{}) {

	// listen for OS signals to shut down the appServer.

	isReady.Store(false)
	logger.Info("received signal to shutdown appServer. Stopping...")

	close(done)
}

func (srv *Server) With(middleware ...MiddlewareFunc) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after appServer has started.")
	}
	srv.middleware = append(srv.middleware, middleware...)
	return srv
}

func (srv *Server) WithOut(middleware ...MiddlewareFunc) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after appServer has started.")
	}
	srv.excludeMiddleware = append(srv.excludeMiddleware, middleware...)
	return srv
}
func (srv *Server) WithStack(stack MiddlewareStack) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after appServer has started.")
	}
	srv.middleware = append(srv.middleware, stack...)
	return srv
}

func (srv *Server) WithOutStack(stack ...MiddlewareFunc) *Server {
	if isRunning.Load() {
		panic("Cannot change middleware after appServer has started.")
	}
	srv.excludeMiddleware = append(srv.excludeMiddleware, stack...)
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
		srv.Shutdown()
		os.Exit(1)
	}
	templates = template.Must(template.ParseGlob(templateDir + "*.html"))
	logger.Info("Templates parsed.", "pattern", templateDir+"*.html")
}

// DataFunc returns a data structure for the template.
type DataFunc func(r *http.Request) interface{}

// WithTLS enables TLS on the appServer.
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

// WithTimeouts adds timeouts to the appServer.
func WithTimeouts(readTimeout, writeTimeout, idleTimeout time.Duration) ServerOptionFunc {
	return func(srv *Server) {
		srv.setTimeouts(readTimeout, writeTimeout, idleTimeout)
	}
}

// WithRateLimit sets rate limiting parameters of the appServer.
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
