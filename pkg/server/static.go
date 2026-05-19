package server

// Static-file serving. Split from server.go for cohesion — both helpers
// here are end-user-facing route helpers for static assets, plus the
// os.Root-backed file server that powers them.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// EnsureTrailingSlash ensures that a directory path ends with a trailing
// slash. Used to normalize directory paths for the http.Dir fallback in
// HandleStatic.
func EnsureTrailingSlash(dir string) string {
	if dir != "" && !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return dir
}

// HandleStatic registers a handler for serving static files from the
// configured static directory. The pattern should typically end with a
// wildcard (e.g., "/static/"). Uses os.Root for secure file access when
// available (Go 1.24+); falls back to http.Dir if the root open fails.
func (srv *Server) HandleStatic(pattern string) {
	if srv.staticRoot == nil && srv.Options.StaticDir != "" {
		staticRoot, err := os.OpenRoot(srv.Options.StaticDir)
		if err != nil {
			logger.Warn("Failed to open static root directory, falling back to http.Dir", "error", err, "dir", srv.Options.StaticDir)
		} else {
			srv.staticRoot = staticRoot
			logger.Info("Static root initialized", "dir", srv.Options.StaticDir)
		}
	}

	srv.registerRoute(pattern)

	if srv.staticRoot != nil {
		srv.mux.Handle(pattern, http.StripPrefix(pattern, srv.rootFileServer()))
		logger.Info("Static file serving using secure os.Root", "pattern", pattern)
	} else {
		staticDir := EnsureTrailingSlash(srv.Options.StaticDir)
		srv.mux.Handle(pattern, http.StripPrefix(pattern, http.FileServer(http.Dir(staticDir))))
		logger.Info("Static file serving using http.Dir", "pattern", pattern, "dir", staticDir)
	}
}

// rootFileServer creates an http.Handler that serves files from srv.staticRoot
// (os.Root-confined). GET and HEAD only — any other method returns 405.
func (srv *Server) rootFileServer() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := filepath.Clean(r.URL.Path)
		if path == "/" {
			path = "index.html"
		}

		file, err := srv.staticRoot.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				logger.Error("Failed to open file", "path", path, "error", err)
			}
			return
		}
		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
	})
}
