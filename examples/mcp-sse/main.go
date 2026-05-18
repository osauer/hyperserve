// Example: MCP over SSE. Run with -mode=server for the listener, or
// -mode=client for a sample SSE consumer that lists tools and calls "echo".
//
//	go run ./examples/mcp-sse -mode=server
//	go run ./examples/mcp-sse -mode=client
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	mode := flag.String("mode", "server", "server | client")
	addr := flag.String("addr", ":8080", "listen address (server) or target host (client)")
	flag.Parse()

	switch *mode {
	case "server":
		runServer(*addr)
	case "client":
		runClient(*addr)
	default:
		log.Fatalf("unknown -mode=%s (want server|client)", *mode)
	}
}

// --- server -----------------------------------------------------------------

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "Echoes the input message" }
func (echoTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "Message to echo",
			},
		},
		"required": []string{"message"},
	}
}
func (echoTool) Execute(params map[string]any) (any, error) {
	message, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message parameter is required")
	}
	return fmt.Sprintf("Echo: %s", message), nil
}

func runServer(addr string) {
	srv, err := serverpkg.NewServer(
		serverpkg.WithAddr(addr),
		serverpkg.WithMCPSupport("sse-example", "1.0.0"),
	)
	if err != nil {
		log.Fatal(err)
	}
	srv.RegisterMCPTool(echoTool{})
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "HyperServe MCP + SSE example")
		fmt.Fprintln(w, "MCP endpoint: /mcp")
		fmt.Fprintln(w, "- HTTP: POST /mcp")
		fmt.Fprintln(w, "- SSE: GET /mcp with Accept: text/event-stream")
		fmt.Fprintln(w, "Routed POSTs require X-SSE-Client-ID + X-SSE-Binding headers.")
	})
	log.Printf("Server starting on %s", addr)
	log.Printf("MCP endpoint: http://localhost%s/mcp", addr)
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

// --- client -----------------------------------------------------------------

type sseClient struct {
	url          string
	clientID     string
	bindingToken string
	body         io.ReadCloser
}

func (c *sseClient) connect() error {
	req, err := http.NewRequest("GET", c.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	c.body = resp.Body

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		after, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(after), &event); err != nil {
			continue
		}
		if id, ok := event["clientId"].(string); ok {
			c.clientID = id
		}
		if tok, ok := event["bindingToken"].(string); ok {
			c.bindingToken = tok
		}
		if c.clientID != "" && c.bindingToken != "" {
			log.Printf("Connected: clientID=%s bindingToken=%s…", c.clientID, c.bindingToken[:8])
			go c.readEvents()
			return nil
		}
	}
	return fmt.Errorf("failed to receive clientId + bindingToken")
}

func (c *sseClient) readEvents() {
	defer c.body.Close()
	scanner := bufio.NewScanner(c.body)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			log.Printf("SSE event: %s", after)
		}
	}
}

func (c *sseClient) send(method string, params any) error {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SSE-Client-ID", c.clientID)
	req.Header.Set("X-SSE-Binding", c.bindingToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Printf("POST %s → %s", method, resp.Status)
	return nil
}

func runClient(addr string) {
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "http://localhost" + host
	} else if !strings.HasPrefix(host, "http") {
		host = "http://" + host
	}
	c := &sseClient{url: host + "/mcp"}

	log.Println("Connecting to MCP over SSE…")
	if err := c.connect(); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Second)

	log.Println("tools/list")
	if err := c.send("tools/list", nil); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Second)

	log.Println("tools/call echo")
	if err := c.send("tools/call", map[string]any{
		"name": "echo",
		"arguments": map[string]any{
			"message": "Hello from SSE client!",
		},
	}); err != nil {
		log.Fatal(err)
	}

	select {}
}
