package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/osauer/hyperserve/go"
)

func main() {

	srv, err := hyperserve.NewServer()
	if err != nil {
		log.Fatal(err)
	}

	// Define a test endpoint
	srv.Handle("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, Chaos Mode!"))
	})

	// Run the server in a goroutine
	go func() {
		srv.Run()
	}()

	// Run the load generator
	time.Sleep(2 * time.Second) // Let the server start
	runLoadTest("http://localhost:8080", 50, 10*time.Second)
}

// Load generator to send concurrent requests to the server
func runLoadTest(url string, concurrentClients int, duration time.Duration) {
	fmt.Println("Starting load test...")
	var wg sync.WaitGroup
	client := &http.Client{}
	results := make(chan string, concurrentClients)

	start := time.Now()

	for i := 0; i < concurrentClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			for time.Since(start) < duration {
				reqStart := time.Now()
				resp, err := client.Get(url)
				elapsed := time.Since(reqStart)

				if err != nil {
					results <- fmt.Sprintf("[Client %d] Error: %v (Response Time: %v)", clientID, err, elapsed)
					continue
				}

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				results <- fmt.Sprintf(
					"[Client %d] Status: %d, Response Time: %v, Body: %s",
					clientID, resp.StatusCode, elapsed, string(body),
				)
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and print results
	for result := range results {
		fmt.Println(result)
	}

	fmt.Println("Load test completed.")
}
