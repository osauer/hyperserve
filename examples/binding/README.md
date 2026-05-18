# Request binding & validation

Three endpoints, same JSON payload, same 400 envelope on validation failure —
different shapes of success path.

| Endpoint            | Helper                       | Success behaviour                                  |
|---------------------|------------------------------|----------------------------------------------------|
| `POST /users/echo`  | `server.JSONEcho[CreateUser]()` | Validates the body and echoes the value back.   |
| `POST /users`       | `server.JSONHandler[In, Out]`   | Validates, runs real logic (assigns ID, lowercases email), returns a different type. |
| `POST /users-manual`| `server.BindJSON` (manual)      | Same shape as the framework path, but built by hand to show what `JSONHandler` hides. |

## Run

```bash
go run ./examples/binding &
curl -s -X POST localhost:8080/users \
     -H 'Content-Type: application/json' \
     -d '{"name":"Ada","email":"Ada@Example.com","age":36,"role":"admin"}'

curl -s -X POST localhost:8080/users/echo \
     -H 'Content-Type: application/json' \
     -d '{"name":"Ada","email":"a@b.com","age":36,"role":"admin"}'
```

## Rule of thumb

If your handler is `func(_, in) (in, nil)`, use `JSONEcho`. The moment you
need to compute, transform, or look anything up, use `JSONHandler`. Reach
for the manual path only when you need custom headers, streaming, or a
non-JSON response shape.
