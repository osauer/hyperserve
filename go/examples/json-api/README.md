# JSON API Example

This example demonstrates how to build a REST API with HyperServe that handles JSON requests and responses. It implements a simple TODO list API with full CRUD operations.

## What This Example Shows

- Building REST endpoints with proper HTTP methods
- Parsing JSON request bodies
- Sending JSON responses
- Error handling with appropriate status codes
- Thread-safe in-memory data storage
- CORS configuration for browser access
- RESTful URL patterns

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | API information |
| GET | `/todos` | List all todos |
| POST | `/todos/create` | Create a new todo |
| GET | `/todos/{id}` | Get a specific todo |
| PUT | `/todos/{id}` | Update a todo |
| DELETE | `/todos/{id}` | Delete a todo |

## Running the Example

```bash
go run main.go
```

The API server will start on http://localhost:8080

## Testing the API

### Using curl

```bash
# Get API info
curl http://localhost:8080/

# List all todos
curl http://localhost:8080/todos

# Create a new todo
curl -X POST http://localhost:8080/todos/create \
  -H "Content-Type: application/json" \
  -d '{"title":"Buy groceries"}'

# Get a specific todo
curl http://localhost:8080/todos/1

# Update a todo
curl -X PUT http://localhost:8080/todos/1 \
  -H "Content-Type: application/json" \
  -d '{"title":"Buy groceries","completed":true}'

# Delete a todo
curl -X DELETE http://localhost:8080/todos/1
```

### Using a REST client

You can also use tools like:
- [Postman](https://www.postman.com/)
- [Insomnia](https://insomnia.rest/)
- [HTTPie](https://httpie.io/)
- VS Code REST Client extension

### From JavaScript

```javascript
// Create a todo
fetch('http://localhost:8080/todos/create', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({title: 'Learn HyperServe'})
})
.then(res => res.json())
.then(todo => console.log('Created:', todo));

// List todos
fetch('http://localhost:8080/todos')
  .then(res => res.json())
  .then(todos => console.log('Todos:', todos));
```

## Key Concepts

### 1. JSON Response Helper

```go
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}
```

This helper ensures consistent JSON responses with proper headers.

### 2. Error Handling

```go
func sendError(w http.ResponseWriter, status int, message string) {
    sendJSON(w, status, map[string]string{"error": message})
}
```

Errors are returned as JSON with appropriate HTTP status codes.

### 3. Request Body Parsing

```go
var input struct {
    Title string `json:"title"`
}
if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
    sendError(w, http.StatusBadRequest, "Invalid JSON")
    return
}
```

Using `json.Decoder` for efficient streaming JSON parsing.

### 4. Thread-Safe Storage

```go
type TodoStore struct {
    mu     sync.RWMutex  // Allows multiple readers
    todos  map[int]*Todo
    nextID int
}
```

The store uses `sync.RWMutex` for concurrent access safety.

### 5. RESTful Routing

```go
// Different HTTP methods on same path
switch r.Method {
case http.MethodGet:    // GET /todos/{id}
case http.MethodPut:    // PUT /todos/{id}
case http.MethodDelete: // DELETE /todos/{id}
}
```

## Common Patterns

### Input Validation

```go
if input.Title == "" {
    sendError(w, http.StatusBadRequest, "Title is required")
    return
}
```

### 404 Handling

```go
todo, exists := store.Get(id)
if !exists {
    sendError(w, http.StatusNotFound, "Todo not found")
    return
}
```

### Status Codes

- 200 OK - Successful GET/PUT
- 201 Created - Successful POST
- 400 Bad Request - Invalid input
- 404 Not Found - Resource doesn't exist
- 405 Method Not Allowed - Wrong HTTP method

## Try These Modifications

1. **Add Pagination**: Implement `?page=1&limit=10` for the list endpoint
2. **Add Filtering**: Allow `?completed=true` to filter todos
3. **Add Validation**: More robust input validation
4. **Add Timestamps**: Track `updated_at` time
5. **Add Search**: Implement `?q=grocery` to search titles

## Production Considerations

This example uses in-memory storage for simplicity. In production, you would:

1. Use a real database (PostgreSQL, MySQL, MongoDB, etc.)
2. Add authentication middleware
3. Implement rate limiting
4. Add request logging
5. Use environment variables for configuration
6. Add input sanitization
7. Implement proper error logging

## What's Next?

Now that you can build APIs, move on to [middleware-basics](../middleware-basics/) to learn about HyperServe's middleware system.