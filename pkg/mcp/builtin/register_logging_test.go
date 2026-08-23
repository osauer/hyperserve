package builtin

import (
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/osauer/hyperserve/pkg/mcp"
	"github.com/osauer/hyperserve/pkg/server"
)

func TestObservabilityPresetDoesNotReplaceGlobalLoggers(t *testing.T) {
	processLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	serverLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	builtinLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	previousProcessLogger := slog.Default()
	previousServerLogger := server.DefaultLogger()
	previousBuiltinLogger := logger
	t.Cleanup(func() {
		logger = previousBuiltinLogger
		server.SetDefaultLogger(previousServerLogger)
		slog.SetDefault(previousProcessLogger)
	})

	slog.SetDefault(processLogger)
	server.SetDefaultLogger(serverLogger)
	logger = builtinLogger

	srv, err := server.NewServer(
		server.WithMCPSupport("logger-isolation", "test", server.MCPObservability()),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	if slog.Default() != processLogger {
		t.Fatal("observability preset replaced slog.Default")
	}
	if server.DefaultLogger() != serverLogger {
		t.Fatal("observability preset replaced the server package logger")
	}
	if logger != builtinLogger {
		t.Fatal("observability preset replaced the builtin package logger")
	}
	if srv.MCPHandler().Logger() == serverLogger {
		t.Fatal("observability preset did not install a handler-owned capture logger")
	}
}

func TestWireLogResourceCapturesOnlyHandlerLogs(t *testing.T) {
	processLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	serverLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	builtinLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlerLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	previousProcessLogger := slog.Default()
	previousServerLogger := server.DefaultLogger()
	previousBuiltinLogger := logger
	t.Cleanup(func() {
		logger = previousBuiltinLogger
		server.SetDefaultLogger(previousServerLogger)
		slog.SetDefault(previousProcessLogger)
	})

	slog.SetDefault(processLogger)
	server.SetDefaultLogger(serverLogger)
	logger = builtinLogger

	handler := mcp.NewHandler(mcp.ServerInfo{Name: "logger-isolation", Version: "test"})
	handler.SetLogger(handlerLogger)
	resource := NewServerLogResource(10)
	wireLogResource(handler, resource)

	handler.Logger().Info("owned MCP log")
	_ = handler.ProcessRequest([]byte("{")) // JSON-RPC engine must use the injected logger too.
	slog.Info("unrelated process log")
	server.DefaultLogger().Info("unrelated server package log")
	logger.Info("unrelated builtin package log")

	result, err := resource.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	raw, ok := result.(string)
	if !ok {
		t.Fatalf("resource payload type = %T, want string", result)
	}
	var payload struct {
		Logs []struct {
			Message string `json:"msg"`
		} `json:"logs"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode resource payload: %v", err)
	}
	messages := make([]string, 0, len(payload.Logs))
	for _, entry := range payload.Logs {
		messages = append(messages, entry.Message)
	}

	for _, want := range []string{"owned MCP log", "Failed to parse JSON-RPC request"} {
		if !slices.Contains(messages, want) {
			t.Errorf("captured messages %v do not contain %q", messages, want)
		}
	}
	for _, unwanted := range []string{
		"unrelated process log",
		"unrelated server package log",
		"unrelated builtin package log",
	} {
		if slices.Contains(messages, unwanted) {
			t.Errorf("captured unrelated message %q in %v", unwanted, messages)
		}
	}
}
