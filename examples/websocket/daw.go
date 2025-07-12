package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/osauer/hyperserve"
)

// JobStatus represents the status of a DAW rendering job
type JobStatus struct {
	ID       string  `json:"id"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Complete bool    `json:"complete"`
}

// ProductionAudioServer simulates a DAW audio rendering server
type ProductionAudioServer struct {
	wsUpgrader websocket.Upgrader
	jobs       map[string]*JobStatus
}

func NewProductionAudioServer() *ProductionAudioServer {
	return &ProductionAudioServer{
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for demo
			},
		},
		jobs: make(map[string]*JobStatus),
	}
}

func (s *ProductionAudioServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		jobID = "default-job"
	}

	log.Printf("WebSocket connection established for job: %s", jobID)

	// Create or get job
	job, exists := s.jobs[jobID]
	if !exists {
		job = &JobStatus{
			ID:       jobID,
			Progress: 0.0,
			Status:   "starting",
			Complete: false,
		}
		s.jobs[jobID] = job
	}

	// Send real-time progress updates
	for {
		// Simulate rendering progress
		if !job.Complete {
			job.Progress += 10.0
			if job.Progress >= 100.0 {
				job.Progress = 100.0
				job.Status = "completed"
				job.Complete = true
			} else {
				job.Status = "rendering audio..."
			}
		}

		// Send progress update
		if err := conn.WriteJSON(job); err != nil {
			log.Printf("Write error: %v", err)
			break
		}

		log.Printf("Sent progress update: %s - %.1f%%", jobID, job.Progress)

		if job.Complete {
			log.Printf("Job %s completed", jobID)
			break
		}

		time.Sleep(500 * time.Millisecond) // Update every 500ms
	}
}

func (s *ProductionAudioServer) StartJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId parameter required", http.StatusBadRequest)
		return
	}

	// Reset job if it exists
	s.jobs[jobID] = &JobStatus{
		ID:       jobID,
		Progress: 0.0,
		Status:   "starting",
		Complete: false,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Job started",
		"jobId":   jobID,
	})
}

func main() {
	srv := hyperserve.NewServer(
		hyperserve.WithPort(8080),
		hyperserve.WithDebug(true),
	)

	// Add middleware stack
	srv.AddMiddleware("*", hyperserve.DefaultMiddleware(srv))

	productionServer := NewProductionAudioServer()

	// WebSocket endpoint for progress updates
	srv.HandleFunc("/ws/daw/production/render", productionServer.HandleWebSocket)

	// REST endpoint to start rendering jobs
	srv.HandleFunc("/api/jobs/start", productionServer.StartJob)

	// Serve static demo page
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "demo.html")
	})

	log.Printf("Starting DAW WebSocket server on port 8080")
	log.Printf("Open http://localhost:8080 in your browser")
	log.Printf("Use /api/jobs/start?jobId=my-job to start a job")
	log.Printf("Connect to /ws/daw/production/render?jobId=my-job for progress updates")
	log.Fatal(srv.ListenAndServe())
}