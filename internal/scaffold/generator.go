package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"go/format"
)

// Generate scaffolds a new HyperServe project and returns the absolute output directory.
func Generate(opts Options) (string, error) {
	if err := opts.normalize(); err != nil {
		return "", err
	}

	info, err := os.Stat(opts.OutputDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat output dir: %w", err)
		}
		if mkErr := os.MkdirAll(opts.OutputDir, 0o755); mkErr != nil {
			return "", fmt.Errorf("create output dir: %w", mkErr)
		}
	} else if !info.IsDir() {
		return "", fmt.Errorf("output path %q is not a directory", opts.OutputDir)
	} else if empty, emptyErr := dirEmpty(opts.OutputDir); emptyErr != nil {
		return "", emptyErr
	} else if !empty && !opts.Force {
		return "", fmt.Errorf("output directory %q is not empty (use --force to override)", opts.OutputDir)
	}

	data := templateData{
		Module:            opts.Module,
		ServiceName:       opts.ServiceName,
		ServiceTitle:      titleize(opts.ServiceName),
		ServiceSlug:       opts.serviceSlug(),
		BinaryName:        opts.serviceSlug(),
		WithMCP:           opts.WithMCP,
		LocalReplace:      filepath.ToSlash(opts.LocalReplace),
		DefaultAddr:       ":8080",
		DefaultHealthAddr: ":9080",
		DefaultRateLimit:  2000,
		DefaultRateBurst:  4000,
	}

	if err := renderTemplates(opts.OutputDir, data); err != nil {
		return "", err
	}

	return opts.OutputDir, nil
}

type templateData struct {
	Module            string
	ServiceName       string
	ServiceTitle      string
	ServiceSlug       string
	BinaryName        string
	WithMCP           bool
	LocalReplace      string
	DefaultAddr       string
	DefaultHealthAddr string
	DefaultRateLimit  int
	DefaultRateBurst  int
}

func renderTemplates(dest string, data templateData) error {
	return fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel("templates", path)
		if err != nil {
			return fmt.Errorf("resolve template path: %w", err)
		}

		if d.IsDir() {
			if relPath == "." {
				return nil
			}
			dirPath := filepath.Join(dest, relPath)
			return os.MkdirAll(dirPath, 0o755)
		}

		contents, readErr := templateFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read template %s: %w", path, readErr)
		}

		buf := bytes.NewBuffer(nil)
		tmpl, parseErr := template.New(filepath.Base(path)).Parse(string(contents))
		if parseErr != nil {
			return fmt.Errorf("parse template %s: %w", path, parseErr)
		}

		if execErr := tmpl.Execute(buf, data); execErr != nil {
			return fmt.Errorf("execute template %s: %w", path, execErr)
		}

		output := buf.Bytes()
		destPath := filepath.Join(dest, relPath)
		if strings.HasSuffix(destPath, ".tmpl") {
			destPath = strings.TrimSuffix(destPath, ".tmpl")
		}

		if strings.HasSuffix(destPath, ".go") {
			formatted, fmtErr := format.Source(output)
			if fmtErr == nil {
				output = formatted
			}
		}

		if err := os.WriteFile(destPath, output, 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", destPath, err)
		}

		return nil
	})
}

func dirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open dir: %w", err)
	}
	defer f.Close()

	_, readErr := f.Readdirnames(1)
	if readErr == nil {
		return false, nil
	}
	if errors.Is(readErr, io.EOF) {
		return true, nil
	}
	return false, fmt.Errorf("read dir entries: %w", readErr)
}

func titleize(input string) string {
	if input == "" {
		return "HyperServe Service"
	}
	parts := strings.FieldsFunc(input, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	if len(parts) == 0 {
		return "HyperServe Service"
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		for j := 1; j < len(runes); j++ {
			runes[j] = unicode.ToLower(runes[j])
		}
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}
