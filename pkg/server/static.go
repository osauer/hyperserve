package server

// Static-file serving. Split from server.go for cohesion — both helpers
// here are end-user-facing route helpers for static assets, plus the
// os.Root-backed file server that powers them.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// HandleStatic registers a handler that serves files only through an
// os.Root confined to Options.StaticDir. It returns an error without
// registering the route if the configured root cannot be opened.
func (srv *Server) HandleStatic(pattern string) error {
	if srv.staticRoot == nil && srv.options.StaticDir != "" {
		staticRoot, err := os.OpenRoot(srv.options.StaticDir)
		if err != nil {
			return fmt.Errorf("open static root %q: %w", srv.options.StaticDir, err)
		}
		srv.staticRoot = staticRoot
		srv.logger.Info("Static root initialized", "dir", srv.options.StaticDir)
	}
	if srv.staticRoot == nil {
		return fmt.Errorf("static directory is not configured")
	}

	srv.registerRoute(pattern)
	srv.mux.Handle(pattern, http.StripPrefix(pattern, srv.rootFileServer()))
	srv.logger.Info("Static file serving using secure os.Root", "pattern", pattern)
	return nil
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
				srv.logger.Error("Failed to open file", "path", path, "error", err)
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
