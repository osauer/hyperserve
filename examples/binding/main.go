// Example: request binding + validation, three ways.
//
// All three endpoints accept the same JSON payload and produce the same
// shape of 400 response. They differ in what they do on success:
//
//	POST /users/echo    — hyperserve.JSONEcho[CreateUser](). Validates the
//	                      body and echoes the validated value back. No
//	                      business logic — useful for webhook acks, dev
//	                      stubs, and "did this payload pass validation?"
//	                      endpoints.
//	POST /users         — hyperserve.JSONHandler. Validates, then runs real
//	                      business logic (here: assigns a server-side ID,
//	                      lowercases the email) and returns a different
//	                      response type.
//	POST /users-manual  — uses the lower-level hyperserve.BindJSON and renders
//	                      the 400 envelope by hand. Useful when you need
//	                      to add headers, stream, or build a custom shape.
//
// Rule of thumb: if your handler would be `func(_, in) (in, nil)`, reach
// for JSONEcho. The moment you need to compute, transform, or look anything
// up, reach for JSONHandler.
//
// Try it:
//
//	go run ./examples/binding &
//	curl -s -X POST localhost:8080/users \
//	     -H 'Content-Type: application/json' \
//	     -d '{"name":"Ada","email":"Ada@Example.com","age":36,"role":"admin"}'
//	curl -s -X POST localhost:8080/users/echo \
//	     -H 'Content-Type: application/json' \
//	     -d '{"name":"Ada","email":"ada@example.com","age":36,"role":"admin"}'
//	curl -s -X POST localhost:8080/users \
//	     -H 'Content-Type: application/json' \
//	     -d '{"name":"A","email":"nope","age":12,"role":"superuser"}'
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/osauer/hyperserve/v2"
)

type CreateUser struct {
	Name  string `json:"name"  validate:"required,min=2,max=64"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age"   validate:"required,min=13,max=120"`
	Role  string `json:"role"  validate:"required,oneof=admin user guest"`
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
	Role  string `json:"role"`
}

// createUser is the only piece of business logic in this example. It maps
// the validated input to the response type, assigns a server-side ID, and
// normalises the email — the kind of work that justifies reaching for
// JSONHandler instead of JSONEcho.
func createUser(_ context.Context, in CreateUser) (User, error) {
	return User{
		ID:    "u_" + in.Name,
		Name:  in.Name,
		Email: strings.ToLower(in.Email),
		Age:   in.Age,
		Role:  in.Role,
	}, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app, err := hyperserve.New(hyperserve.WithAddr(":8080"))
	if err != nil {
		log.Fatal(err)
	}

	// Shortest form: validate-and-echo. JSONEcho is the right tool when
	// the handler body would just be `return in, nil` — there's no
	// business logic, only "did this payload validate?".
	app.POST("/users/echo", hyperserve.JSONEcho[CreateUser]())

	// High-level: hyperserve.JSONHandler does bind + validate + JSON respond.
	// Use it when the response is genuinely different from the input —
	// here, we assign an ID and normalise the email.
	app.POST("/users", hyperserve.JSONHandler(createUser))

	// Low-level: same behaviour, hand-rolled. Use this shape when you
	// need to set custom headers, write a non-JSON body, or stream.
	app.POST("/users-manual", func(w http.ResponseWriter, r *http.Request) {
		var in CreateUser
		if err := hyperserve.BindJSON(r, &in); err != nil {
			if verr, ok := errors.AsType[*hyperserve.ValidationError](err); ok {
				writeValidationError(w, verr)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := createUser(r.Context(), in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	log.Println("listening on :8080")
	log.Fatal(app.Run(ctx))
}

// writeValidationError mirrors the per-field 400 envelope that
// hyperserve.JSONHandler renders automatically. Kept here so the low-level
// example shows what's normally hidden.
func writeValidationError(w http.ResponseWriter, verr *hyperserve.ValidationError) {
	type field struct {
		Field   string `json:"field"`
		Tag     string `json:"tag,omitempty"`
		Param   string `json:"param,omitempty"`
		Message string `json:"message"`
	}
	payload := struct {
		Error  string  `json:"error"`
		Fields []field `json:"fields"`
	}{
		Error:  "validation failed",
		Fields: make([]field, 0, len(verr.Fields)),
	}
	for _, f := range verr.Fields {
		payload.Fields = append(payload.Fields, field{
			Field:   f.Field,
			Tag:     f.Tag,
			Param:   f.Param,
			Message: f.Message,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(payload)
}
