package hyperserve

import (
	"sync"
	"testing"
)

func TestMetricsConcurrentTotalsAndReset(t *testing.T) {
	var srv Server
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 10000 {
				srv.AddMetrics(1, 7)
				srv.AddMetrics(0, -2)
			}
		})
	}
	wg.Wait()
	if got := srv.TotalRequests(); got != 320000 {
		t.Fatalf("requests = %d", got)
	}
	if got := srv.TotalResponseTime(); got != 1600000 {
		t.Fatalf("time = %d", got)
	}
	srv.SetMetrics(12, -3)
	srv.AddMetrics(2, 1)
	if srv.TotalRequests() != 14 || srv.TotalResponseTime() != -2 {
		t.Fatal("reset/add lost totals")
	}
}

func TestMetricsConcurrentReset(t *testing.T) {
	var srv Server
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				srv.AddMetrics(1, 1)
				_ = srv.TotalRequests()
				_ = srv.TotalResponseTime()
			}
		})
	}
	wg.Go(func() {
		for range 1000 {
			srv.SetMetrics(0, 0)
		}
	})
	wg.Wait()
	srv.SetMetrics(0, 0)
	srv.AddMetrics(1, 2)
	if srv.TotalRequests() != 1 || srv.TotalResponseTime() != 2 {
		t.Fatal("old generation survived reset")
	}
}
