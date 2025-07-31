package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// SSEClient demonstrates connecting to MCP via SSE
type SSEClient struct {
	url      string
	clientID string
}

// Connect establishes an SSE connection and retrieves the client ID
func (c *SSEClient) Connect() error {
	req, err := http.NewRequest("GET", c.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	// Read SSE events
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				if clientID, ok := event["clientId"].(string); ok {
					c.clientID = clientID
					log.Printf("Connected with client ID: %s", clientID)
					go c.keepReading(resp.Body)
					return nil
				}
			}
		}
	}
	return fmt.Errorf("failed to get client ID")
}

// keepReading continues reading SSE events
func (c *SSEClient) keepReading(body io.ReadCloser) {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			log.Printf("SSE Response: %s", data)
		}
	}
}

// SendRequest sends an MCP request using the SSE client ID
func (c *SSEClient) SendRequest(method string, params interface{}) error {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SSE-Client-ID", c.clientID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("Request sent, status: %s", resp.Status)
	return nil
}

func main() {
	client := &SSEClient{
		url: "http://localhost:8080/mcp",
	}

	// Connect via SSE
	log.Println("Connecting to MCP via SSE...")
	if err := client.Connect(); err != nil {
		log.Fatal(err)
	}

	// Wait a moment for connection to establish
	time.Sleep(time.Second)

	// Send some requests
	log.Println("Listing available tools...")
	client.SendRequest("tools/list", nil)

	time.Sleep(time.Second)

	log.Println("Calling echo tool...")
	client.SendRequest("tools/call", map[string]interface{}{
		"name": "echo",
		"arguments": map[string]interface{}{
			"message": "Hello from SSE client!",
		},
	})

	// Keep the client running
	select {}
}