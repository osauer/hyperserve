// Command load runs a bounded concurrent HTTP workload using only the Go
// standard library. It is intentionally small: the goal is a reproducible
// comparison harness, not a general-purpose load-testing product.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var latencyBounds = []time.Duration{
	100 * time.Microsecond,
	250 * time.Microsecond,
	500 * time.Microsecond,
	time.Millisecond,
	2500 * time.Microsecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2500 * time.Millisecond,
	5 * time.Second,
}

type workerStats struct {
	requests      int64
	succeeded     int64
	failed        int64
	responseBytes int64
	totalLatency  time.Duration
	latency       []int64
	statuses      map[int]int64
	firstError    string
}

func main() {
	target := flag.String("url", "", "HTTP or HTTPS endpoint to request")
	duration := flag.Duration("duration", 10*time.Second, "measurement duration")
	workers := flag.Int("workers", 32, "number of concurrent request workers")
	token := flag.String("bearer-token", "", "optional bearer token")
	flag.Parse()

	if err := validateInputs(*target, *duration, *workers); err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(2)
	}

	transport := &http.Transport{
		MaxIdleConns:        *workers,
		MaxIdleConnsPerHost: *workers,
		MaxConnsPerHost:     *workers,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	started := time.Now()
	stats := run(ctx, client, *target, *token, *workers)
	elapsed := time.Since(started)

	printReport(*target, *duration, *workers, elapsed, stats)
	if stats.succeeded == 0 || stats.failed != 0 {
		os.Exit(1)
	}
}

func validateInputs(target string, duration time.Duration, workers int) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("-url must be an absolute HTTP or HTTPS URL")
	}
	if duration <= 0 {
		return fmt.Errorf("-duration must be positive")
	}
	if workers <= 0 {
		return fmt.Errorf("-workers must be positive")
	}
	return nil
}

func run(ctx context.Context, client *http.Client, target, token string, workers int) workerStats {
	results := make(chan workerStats, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			results <- runWorker(ctx, client, target, token)
		})
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	combined := newWorkerStats()
	for result := range results {
		combined.merge(result)
	}
	return combined
}

func runWorker(ctx context.Context, client *http.Client, target, token string) workerStats {
	stats := newWorkerStats()
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			stats.recordError(err)
			return stats
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		started := time.Now()
		resp, err := client.Do(req)
		latency := time.Since(started)
		if err != nil {
			if ctx.Err() == nil {
				stats.recordError(err)
			}
			continue
		}
		responseBytes, readErr := io.Copy(io.Discard, resp.Body)
		closeErr := resp.Body.Close()
		stats.recordResponse(resp.StatusCode, responseBytes, latency, errors.Join(readErr, closeErr))
	}
	return stats
}

func newWorkerStats() workerStats {
	return workerStats{
		latency:  make([]int64, len(latencyBounds)+1),
		statuses: make(map[int]int64),
	}
}

func (s *workerStats) recordResponse(status int, responseBytes int64, latency time.Duration, responseErr error) {
	s.requests++
	s.responseBytes += responseBytes
	s.totalLatency += latency
	s.statuses[status]++
	index := sort.Search(len(latencyBounds), func(i int) bool { return latency <= latencyBounds[i] })
	s.latency[index]++
	if responseErr != nil {
		s.failed++
		s.keepFirstError(responseErr)
	} else if status >= 200 && status < 300 {
		s.succeeded++
	} else {
		s.failed++
	}
}

func (s *workerStats) recordError(err error) {
	s.requests++
	s.failed++
	s.keepFirstError(err)
}

func (s *workerStats) keepFirstError(err error) {
	if s.firstError == "" {
		s.firstError = err.Error()
	}
}

func (s *workerStats) merge(other workerStats) {
	s.requests += other.requests
	s.succeeded += other.succeeded
	s.failed += other.failed
	s.responseBytes += other.responseBytes
	s.totalLatency += other.totalLatency
	for i, count := range other.latency {
		s.latency[i] += count
	}
	for status, count := range other.statuses {
		s.statuses[status] += count
	}
	if s.firstError == "" {
		s.firstError = other.firstError
	}
}

func (s workerStats) percentile(q float64) string {
	observations := s.responseCount()
	if observations == 0 {
		return "n/a"
	}
	want := int64(float64(observations)*q + 0.999999)
	var seen int64
	for i, count := range s.latency {
		seen += count
		if seen < want {
			continue
		}
		if i == len(latencyBounds) {
			return ">" + latencyBounds[len(latencyBounds)-1].String()
		}
		return "≤" + latencyBounds[i].String()
	}
	return "n/a"
}

func (s workerStats) responseCount() int64 {
	var count int64
	for _, bucket := range s.latency {
		count += bucket
	}
	return count
}

func printReport(target string, planned time.Duration, workers int, elapsed time.Duration, stats workerStats) {
	mean := time.Duration(0)
	if responses := stats.responseCount(); responses > 0 {
		mean = stats.totalLatency / time.Duration(responses)
	}
	rate := float64(stats.requests) / elapsed.Seconds()

	fmt.Printf("Target: %s\n", target)
	fmt.Printf("Planned duration: %s\n", planned)
	fmt.Printf("Measured duration: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Workers (connections): %d\n", workers)
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("Requests: %d (%d succeeded, %d failed)\n", stats.requests, stats.succeeded, stats.failed)
	fmt.Printf("Requests/sec: %.2f\n", rate)
	fmt.Printf("Response bytes: %d\n", stats.responseBytes)
	fmt.Printf("Latency mean: %s\n", mean)
	fmt.Printf("Latency p50: %s\n", stats.percentile(0.50))
	fmt.Printf("Latency p95: %s\n", stats.percentile(0.95))
	fmt.Printf("Latency p99: %s\n", stats.percentile(0.99))

	statusCodes := make([]int, 0, len(stats.statuses))
	for status := range stats.statuses {
		statusCodes = append(statusCodes, status)
	}
	sort.Ints(statusCodes)
	parts := make([]string, 0, len(statusCodes))
	for _, status := range statusCodes {
		parts = append(parts, fmt.Sprintf("%d=%d", status, stats.statuses[status]))
	}
	fmt.Printf("Statuses: %s\n", strings.Join(parts, ", "))
	if stats.firstError != "" {
		fmt.Printf("First error: %s\n", stats.firstError)
	}
}
