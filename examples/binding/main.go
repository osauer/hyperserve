// Example: request binding + validation, two ways.
//
// Both endpoints accept the same JSON payload and produce the same shape of
// 200 / 400 response. The difference is mechanical, not behavioural:
//
//   POST /users         — uses server.JSONHandler. One function, business
//                         logic only. Bind, validate, render are wrapped.
//   POST /users-manual  — uses the lower-level server.BindJSON and renders
//                         the 400 envelope by hand. Useful when you need
//                         to add headers, stream, or build a custom shape.
//
// Try it:
//
//	go run ./examples/binding &
//	curl -s -X POST localhost:8080/users \
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

	server "github.com/osauer/hyperserve/pkg/server"
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

// createUser is the only piece of business logic in this example. Both
// handlers below delegate here.
func createUser(_ context.Context, in CreateUser) (User, error) {
	return User{
		ID:    "u_" + in.Name,
		Name:  in.Name,
		Email: in.Email,
		Age:   in.Age,
		Role:  in.Role,
	}, nil
}

func main() {
	srv, err := server.NewServer(server.WithAddr(":8080"))
	if err != nil {
		log.Fatal(err)
	}

	// High-level: server.JSONHandler does bind + validate + JSON respond.
	// Method-prefix in the pattern is Go 1.22+ http.ServeMux syntax.
	srv.HandleFunc("POST /users", server.JSONHandler(createUser))

	// Low-level: same behaviour, hand-rolled. Use this shape when you
	// need to set custom headers, write a non-JSON body, or stream.
	srv.HandleFunc("POST /users-manual", func(w http.ResponseWriter, r *http.Request) {
		var in CreateUser
		if err := server.BindJSON(r, &in); err != nil {
			var verr *server.ValidationError
			if errors.As(err, &verr) {
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
	log.Fatal(srv.Run())
}

// writeValidationError mirrors the per-field 400 envelope that
// server.JSONHandler renders automatically. Kept here so the low-level
// example shows what's normally hidden.
func writeValidationError(w http.ResponseWriter, verr *server.ValidationError) {
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
