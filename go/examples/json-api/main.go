package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/osauer/hyperserve/go"
)

// Todo represents a task in our TODO list
type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

// TodoStore manages our in-memory TODO storage
type TodoStore struct {
	mu     sync.RWMutex
	todos  map[int]*Todo
	nextID int
}

// NewTodoStore creates a new TODO store
func NewTodoStore() *TodoStore {
	return &TodoStore{
		todos:  make(map[int]*Todo),
		nextID: 1,
	}
}

// List returns all todos
func (s *TodoStore) List() []*Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todos := make([]*Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		todos = append(todos, todo)
	}
	return todos
}

// Get returns a specific todo by ID
func (s *TodoStore) Get(id int) (*Todo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todo, exists := s.todos[id]
	return todo, exists
}

// Create adds a new todo
func (s *TodoStore) Create(title string) *Todo {
	s.mu.Lock()
	defer s.mu.Unlock()

	todo := &Todo{
		ID:        s.nextID,
		Title:     title,
		Completed: false,
		CreatedAt: time.Now(),
	}
	s.todos[s.nextID] = todo
	s.nextID++

	return todo
}

// Update modifies an existing todo
func (s *TodoStore) Update(id int, title string, completed bool) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	todo, exists := s.todos[id]
	if !exists {
		return nil, false
	}

	todo.Title = title
	todo.Completed = completed
	return todo, true
}

// Delete removes a todo
func (s *TodoStore) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.todos[id]
	if exists {
		delete(s.todos, id)
	}
	return exists
}

// Helper function to send JSON responses
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Helper function to send error responses
func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}

// CORS middleware for API access from browsers
func corsMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func main() {
	// Create our todo store
	store := NewTodoStore()

	// Add some sample todos
	store.Create("Learn HyperServe")
	store.Create("Build a REST API")
	store.Create("Add authentication")

	// Create server
	server, err := hyperserve.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Add CORS middleware for API access from browsers
	server.AddMiddleware("*", corsMiddleware)

	// GET /todos - List all todos
	server.HandleFunc("/todos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		todos := store.List()
		sendJSON(w, http.StatusOK, todos)
	})

	// POST /todos - Create a new todo
	server.HandleFunc("/todos/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var input struct {
			Title string `json:"title"`
		}

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		if input.Title == "" {
			sendError(w, http.StatusBadRequest, "Title is required")
			return
		}

		todo := store.Create(input.Title)
		sendJSON(w, http.StatusCreated, todo)
	})

	// Handler for specific todo operations
	todoHandler := func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from URL path
		path := strings.TrimPrefix(r.URL.Path, "/todos/")
		if path == "" || path == r.URL.Path {
			sendError(w, http.StatusBadRequest, "Invalid todo ID")
			return
		}

		id, err := strconv.Atoi(path)
		if err != nil {
			sendError(w, http.StatusBadRequest, "Invalid todo ID")
			return
		}

		switch r.Method {
		case http.MethodGet:
			// GET /todos/{id} - Get a specific todo
			todo, exists := store.Get(id)
			if !exists {
				sendError(w, http.StatusNotFound, "Todo not found")
				return
			}
			sendJSON(w, http.StatusOK, todo)

		case http.MethodPut:
			// PUT /todos/{id} - Update a todo
			var input struct {
				Title     string `json:"title"`
				Completed bool   `json:"completed"`
			}

			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				sendError(w, http.StatusBadRequest, "Invalid JSON")
				return
			}

			if input.Title == "" {
				sendError(w, http.StatusBadRequest, "Title is required")
				return
			}

			todo, exists := store.Update(id, input.Title, input.Completed)
			if !exists {
				sendError(w, http.StatusNotFound, "Todo not found")
				return
			}
			sendJSON(w, http.StatusOK, todo)

		case http.MethodDelete:
			// DELETE /todos/{id} - Delete a todo
			if !store.Delete(id) {
				sendError(w, http.StatusNotFound, "Todo not found")
				return
			}
			sendJSON(w, http.StatusOK, map[string]string{"message": "Todo deleted"})

		default:
			sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}

	// Register the handler for all /todos/{id} routes
	server.Handle("/todos/", http.HandlerFunc(todoHandler))

	// Root endpoint with API information
	server.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]interface{}{
			"service": "HyperServe TODO API",
			"version": "1.0",
			"endpoints": map[string]string{
				"GET /":                "API information",
				"GET /todos":           "List all todos",
				"POST /todos/create":   "Create a new todo",
				"GET /todos/{id}":      "Get a specific todo",
				"PUT /todos/{id}":      "Update a todo",
				"DELETE /todos/{id}":   "Delete a todo",
			},
		}
		sendJSON(w, http.StatusOK, info)
	})

	// Start the server
	fmt.Println("TODO API Server starting on http://localhost:8080")
	fmt.Println("\nAPI Endpoints:")
	fmt.Println("  GET    /              - API information")
	fmt.Println("  GET    /todos         - List all todos")
	fmt.Println("  POST   /todos/create  - Create a new todo")
	fmt.Println("  GET    /todos/{id}    - Get a specific todo")
	fmt.Println("  PUT    /todos/{id}    - Update a todo")
	fmt.Println("  DELETE /todos/{id}    - Delete a todo")
	fmt.Println("\nPress Ctrl+C to stop")

	if err := server.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}