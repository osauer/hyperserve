// Synthetic server shared by the SDK and optional downstream consumer checks.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/mcp"
)

type input struct {
	Message string `json:"message"`
}

type output struct {
	Message string `json:"message"`
}

func main() {
	version := flag.String("protocol-version", mcp.DefaultProtocolVersion, "initialize-era MCP version")
	flag.Parse()
	server, err := hyperserve.New(
		hyperserve.WithMCPSupport("consumer-check", "1", mcp.OverStdio()),
		hyperserve.WithMCPProtocolVersion(*version),
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, tool := range []mcp.Tool{
		mcp.NewTypedTool("echo", "Return synthetic input", func(_ context.Context, in input) (output, error) {
			return output{Message: in.Message}, nil
		}),
		mcp.NewTypedTool("fail", "Return a synthetic domain error", func(context.Context, struct{}) (output, error) {
			return output{}, mcp.ToolError("synthetic rejection")
		}),
	} {
		if err := server.RegisterMCPTool(tool); err != nil {
			log.Fatal(err)
		}
	}
	if err := server.RunStdio(); err != nil {
		log.Fatal(err)
	}
}
