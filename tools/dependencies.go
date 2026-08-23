//go:build tools

// Package tools keeps developer-only dependencies outside HyperServe's
// shipped module graph.
package tools

import _ "github.com/osauer/hyperserve/pkg/server"
