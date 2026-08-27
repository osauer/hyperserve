package hyperserve

// Template-rendering helpers. Split from server.go for cohesion — the
// template machinery is self-contained (parsing, root walking, dynamic
// and static render handlers) and lives apart from the lifecycle code
// that powers the rest of the file.

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DataFunc is a function type that generates data for template rendering.
// It receives the current HTTP request and returns data to be passed to the
// template.
type DataFunc func(r *http.Request) any

// openTemplateRoot lazily opens TemplateDir under an os.Root so template
// loads can't escape the configured directory. Failure is logged but not
// fatal — handlers that need templates will report a clearer error later.
func openTemplateRoot(srv *Server) {
	if srv.options.TemplateDir == "" {
		return
	}
	templateRoot, err := os.OpenRoot(srv.options.TemplateDir)
	if err != nil {
		srv.logger.Debug("Failed to open template root directory", "error", err, "dir", srv.options.TemplateDir)
		return
	}
	srv.templateRoot = templateRoot
	srv.logger.Debug("Template root initialized", "dir", srv.options.TemplateDir)
}

// HandleFuncDynamic registers a handler that renders templates with dynamic
// data. The dataFunc is called for each request to generate the data passed
// to the template. Returns an error if template parsing fails.
func (srv *Server) HandleFuncDynamic(pattern, tmplName string, dataFunc DataFunc) error {
	if err := srv.parseTemplates(); err != nil {
		srv.logger.Error("Failed to parse templates", "error", err)
		return err
	}

	srv.registerRoute(pattern)

	if srv.templates != nil && srv.templates.Lookup(tmplName) == nil {
		return fmt.Errorf("template %s not found", tmplName)
	}

	srv.mux.HandleFunc(pattern,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")

			data := dataFunc(r)
			if err := srv.templates.ExecuteTemplate(w, tmplName, data); err != nil {
				srv.logger.Error("Failed to execute template", "template", tmplName, "error", err)
				http.Error(w, "Error rendering template", http.StatusInternalServerError)
				return
			}
		})
	return nil
}

// HandleTemplate registers a handler that renders a specific template with
// static data. Unlike HandleFuncDynamic, the data is provided once at
// registration time. Returns an error if template parsing fails.
func (srv *Server) HandleTemplate(pattern, t string, data any) error {
	if err := srv.parseTemplates(); err != nil {
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	srv.registerRoute(pattern)

	if srv.templates != nil && srv.templates.Lookup(t) == nil {
		return fmt.Errorf("template %s not found", t)
	}

	srv.mux.HandleFunc(pattern, srv.templateHandler(t, data))
	return nil
}

func (srv *Server) parseTemplates() error {
	srv.templatesMu.Lock()
	defer srv.templatesMu.Unlock()

	if srv.templates != nil {
		return nil
	}

	if srv.templateRoot != nil {
		// Use secure os.Root for template parsing (Go 1.24+)
		tmpl := template.New("root")

		templateFiles, err := srv.listTemplateFiles()
		if err != nil {
			return fmt.Errorf("failed to list template files: %w", err)
		}

		for _, filename := range templateFiles {
			if strings.HasSuffix(filename, ".html") {
				file, err := srv.templateRoot.Open(filename)
				if err != nil {
					srv.logger.Error("Failed to open template file", "file", filename, "error", err)
					continue
				}

				content, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					srv.logger.Error("Failed to read template file", "file", filename, "error", err)
					continue
				}

				_, err = tmpl.New(filename).Parse(string(content))
				if err != nil {
					srv.logger.Error("Failed to parse template", "file", filename, "error", err)
					return fmt.Errorf("failed to parse template %s: %w", filename, err)
				}
			}
		}

		srv.templates = tmpl
		srv.logger.Info("Templates parsed using secure os.Root", "count", len(tmpl.Templates())-1) // -1 for root template
		return nil
	}

	// Fallback to traditional template parsing
	templateDir := srv.options.TemplateDir
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		wd, _ := os.Getwd()
		ad, _ := filepath.Abs(templateDir)
		return fmt.Errorf("template directory not found. working-dir %s abs-path: %s, error %w", wd, ad, err)
	}

	tmpl, err := template.ParseGlob(filepath.Join(templateDir, "*.html"))
	if err != nil {
		srv.logger.Error("Failed to parse templates", "error", err)
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	srv.templates = tmpl
	srv.logger.Info("Templates parsed.", "pattern", filepath.Join(templateDir, "*.html"))
	return nil
}

// listTemplateFiles lists all files in the template root directory.
// os.Root doesn't expose ReadDir, so we list via the regular os package and
// re-validate each entry by opening it through srv.templateRoot — files
// outside the root simply fail to open and are dropped.
func (srv *Server) listTemplateFiles() ([]string, error) {
	var files []string

	entries, err := os.ReadDir(srv.options.TemplateDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			file, err := srv.templateRoot.Open(entry.Name())
			if err == nil {
				file.Close()
				files = append(files, entry.Name())
			}
		}
	}

	return files, nil
}
