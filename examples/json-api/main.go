package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

// Todo represents a task in the API.
type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

type todoInput struct {
	Title     string `json:"title" validate:"required,min=1,max=200"`
	Completed bool   `json:"completed"`
}

// TodoStore is intentionally in-memory so the example focuses on HTTP shape.
type TodoStore struct {
	mu     sync.RWMutex
	todos  map[int]*Todo
	nextID int
}

func NewTodoStore() *TodoStore {
	return &TodoStore{
		todos:  make(map[int]*Todo),
		nextID: 1,
	}
}

func (s *TodoStore) List() []*Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todos := make([]*Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		todos = append(todos, todo)
	}
	return todos
}

func (s *TodoStore) Get(id int) (*Todo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todo, exists := s.todos[id]
	return todo, exists
}

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

func (s *TodoStore) Update(id int, input todoInput) (*Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	todo, exists := s.todos[id]
	if !exists {
		return nil, false
	}
	todo.Title = input.Title
	todo.Completed = input.Completed
	return todo, true
}

func (s *TodoStore) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.todos[id]; !exists {
		return false
	}
	delete(s.todos, id)
	return true
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, map[string]string{"error": message})
}

func todoID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	return id, err == nil
}

func main() {
	store := NewTodoStore()
	store.Create("Learn HyperServe")
	store.Create("Build a REST API")
	store.Create("Add authentication")

	srv, err := serverpkg.NewServer(
		serverpkg.WithCORS(&serverpkg.CORSOptions{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type"},
		}),
	)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	srv.GET("/", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, http.StatusOK, map[string]any{
			"service": "HyperServe TODO API",
			"version": "1.0",
			"endpoints": map[string]string{
				"GET /":              "API information",
				"GET /todos":         "List all todos",
				"POST /todos":        "Create a new todo",
				"GET /todos/{id}":    "Get a specific todo",
				"PUT /todos/{id}":    "Update a todo",
				"DELETE /todos/{id}": "Delete a todo",
			},
		})
	})

	srv.GET("/todos", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, http.StatusOK, store.List())
	})

	srv.POST("/todos", func(w http.ResponseWriter, r *http.Request) {
		var input todoInput
		if err := serverpkg.BindJSON(r, &input); err != nil {
			sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, store.Create(input.Title))
	})

	srv.GET("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := todoID(r)
		if !ok {
			sendError(w, http.StatusBadRequest, "invalid todo ID")
			return
		}
		todo, exists := store.Get(id)
		if !exists {
			sendError(w, http.StatusNotFound, "todo not found")
			return
		}
		sendJSON(w, http.StatusOK, todo)
	})

	srv.PUT("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := todoID(r)
		if !ok {
			sendError(w, http.StatusBadRequest, "invalid todo ID")
			return
		}
		var input todoInput
		if err := serverpkg.BindJSON(r, &input); err != nil {
			sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		todo, exists := store.Update(id, input)
		if !exists {
			sendError(w, http.StatusNotFound, "todo not found")
			return
		}
		sendJSON(w, http.StatusOK, todo)
	})

	srv.DELETE("/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := todoID(r)
		if !ok {
			sendError(w, http.StatusBadRequest, "invalid todo ID")
			return
		}
		if !store.Delete(id) {
			sendError(w, http.StatusNotFound, "todo not found")
			return
		}
		sendJSON(w, http.StatusOK, map[string]string{"message": "todo deleted"})
	})

	fmt.Println("TODO API Server starting on http://localhost:8080")
	fmt.Println("\nAPI Endpoints:")
	fmt.Println("  GET    /            - API information")
	fmt.Println("  GET    /todos       - List all todos")
	fmt.Println("  POST   /todos       - Create a new todo")
	fmt.Println("  GET    /todos/{id}  - Get a specific todo")
	fmt.Println("  PUT    /todos/{id}  - Update a todo")
	fmt.Println("  DELETE /todos/{id}  - Delete a todo")
	fmt.Println("\nPress Ctrl+C to stop")

	if err := srv.Run(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
