package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	server "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	var (
		port    = flag.Int("port", 8080, "Port to listen on")
		mcp     = flag.Bool("mcp", true, "Enable MCP support")
		verbose = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	// Create server with options
	addr := fmt.Sprintf(":%d", *port)
	var opts []server.ServerOptionFunc

	if *mcp {
		opts = append(opts,
			server.WithMCPSupport("HyperServe Go", "1.0.0"),
			server.WithMCPBuiltinTools(true),
			server.WithMCPBuiltinResources(true),
		)
	}

	if *verbose {
		opts = append(opts, server.WithDebugMode())
	}

	opts = append(opts, server.WithAddr(addr))

	// Create server
	srv, err := server.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}

	// Add routes
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>HyperServe Go</title></head>
<body>
<h1>HyperServe Go Implementation</h1>
<p>Server is running!</p>
</body>
</html>`))
	})

	srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy","service":"hyperserve-go"}`))
	})

	// Start server
	fmt.Printf("HyperServe Go server listening on %s\n", addr)
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
