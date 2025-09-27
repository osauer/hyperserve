package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/osauer/hyperserve/internal/scaffold"
)

func main() {
	var (
		module       = flag.String("module", "", "Go module path for the new project (required)")
		name         = flag.String("name", "", "Service name (defaults to the last segment of the module path)")
		out          = flag.String("out", "", "Output directory (defaults to service name)")
		withMCP      = flag.Bool("with-mcp", true, "Generate with Model Context Protocol support enabled")
		force        = flag.Bool("force", false, "Allow writing into a non-empty directory")
		localReplace = flag.String("local-replace", "", "Add a replace directive pointing to a local hyperserve checkout")
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "HyperServe scaffolding CLI\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: hyperserve-init --module=github.com/acme/service [flags]\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if strings.TrimSpace(*module) == "" {
		flag.Usage()
		os.Exit(1)
	}

	replacePath := strings.TrimSpace(*localReplace)
	if replacePath != "" {
		abs, err := filepath.Abs(replacePath)
		if err != nil {
			log.Fatalf("resolve local-replace: %v", err)
		}
		replacePath = abs
	}

	opts := scaffold.Options{
		Module:       *module,
		ServiceName:  *name,
		OutputDir:    *out,
		WithMCP:      *withMCP,
		Force:        *force,
		LocalReplace: replacePath,
	}

	dest, err := scaffold.Generate(opts)
	if err != nil {
		log.Fatalf("generate project: %v", err)
	}

	rel := dest
	if cwd, err := os.Getwd(); err == nil {
		if r, relErr := filepath.Rel(cwd, dest); relErr == nil {
			rel = r
		}
	}

	fmt.Printf("✅ Generated HyperServe project at %s\n", rel)
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", rel)
	fmt.Println("  go mod tidy")
	fmt.Println("  go run ./cmd/server")
}
