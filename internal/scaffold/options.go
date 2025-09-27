package scaffold

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Options controls scaffold generation.
type Options struct {
	Module       string
	ServiceName  string
	OutputDir    string
	WithMCP      bool
	Force        bool
	LocalReplace string
}

func (o *Options) normalize() error {
	o.Module = strings.TrimSpace(o.Module)
	o.ServiceName = strings.TrimSpace(o.ServiceName)
	o.OutputDir = strings.TrimSpace(o.OutputDir)
	o.LocalReplace = strings.TrimSpace(o.LocalReplace)

	if o.Module == "" {
		return errors.New("module path is required")
	}
	if strings.Contains(o.Module, " ") {
		return fmt.Errorf("module path %q must not contain spaces", o.Module)
	}

	if o.ServiceName == "" {
		parts := strings.Split(o.Module, "/")
		o.ServiceName = parts[len(parts)-1]
	}

	if o.OutputDir == "" {
		o.OutputDir = o.ServiceName
	}

	if !filepath.IsAbs(o.OutputDir) {
		abs, err := filepath.Abs(o.OutputDir)
		if err != nil {
			return fmt.Errorf("resolve output dir: %w", err)
		}
		o.OutputDir = abs
	}

	return nil
}

func (o Options) serviceSlug() string {
	return slugify(o.ServiceName)
}

func slugify(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(
		" ", "-",
		"_", "-",
		".", "-",
		"--", "-",
	)
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "service"
	}
	return value
}
