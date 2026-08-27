# JSON API

This example puts method-aware routes, bounded JSON input, validation, error
responses, and concurrent in-memory state into one small TODO API. Read the
focused [binding example](../binding/) first if you only need to compare
`JSONHandler` with manual binding.

## Run

From the repository root:

```sh
go run ./examples/json-api
```

| Method | Path | Result |
|---|---|---|
| `GET` | `/todos` | List todos. |
| `POST` | `/todos` | Create a todo from validated JSON. |
| `GET` | `/todos/{id}` | Read one todo. |
| `PUT` | `/todos/{id}` | Replace one todo. |
| `DELETE` | `/todos/{id}` | Delete one todo. |

Try one write and one read:

```sh
curl -sS -X POST http://localhost:8080/todos \
  -H 'Content-Type: application/json' \
  -d '{"title":"Buy groceries"}'

curl -sS http://localhost:8080/todos
```

## What to notice

The method helpers rely on Go's `ServeMux`, so the mux rejects a wrong method
before the handler runs:

```go
srv.GET("/todos/{id}", getTodo)
srv.PUT("/todos/{id}", updateTodo)
srv.DELETE("/todos/{id}", deleteTodo)
```

Writes use `BindJSON` rather than an unbounded decoder:

```go
type todoInput struct {
    Title string `json:"title" validate:"required,min=1,max=200"`
}

var input todoInput
if err := server.BindJSON(r, &input); err != nil {
    sendError(w, http.StatusBadRequest, err.Error())
    return
}
```

`BindJSON` limits the body to 1 MiB, rejects unknown fields, and evaluates the
validation tags. The example keeps a manual response envelope because a CRUD
API needs status codes and error shapes that differ from `JSONHandler`'s short
path.

The store uses `sync.RWMutex` because handlers run concurrently. It is example
state, not a persistence recommendation. Authentication, authorization, and
rate limiting are intentionally absent so the HTTP and data boundaries remain
visible.
