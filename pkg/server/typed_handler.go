package server

// Typed JSON handler.
//
// JSONHandler absorbs the bind + validate + respond boilerplate that every
// JSON endpoint otherwise writes by hand: decode the body into a typed
// struct, validate with `validate:"..."` rules, call the business function,
// JSON-encode the result. The lower-level BindJSON / Validate / manual
// encoding path is still available for handlers that need more control
// (streaming, custom envelopes, multi-step responses).
//
// Error model:
//
//   - *ValidationError      → 400 with a per-field envelope (Field, Tag,
//     Param, Message).
//   - other bind errors     → 400 with {"error": err.Error()}.
//   - error implementing
//     `HTTPStatus() int`    → that status with {"error": err.Error()}.
//   - everything else       → 500 with a generic message; the original
//     error string is not leaked to the client.
//
// 204 No Content is written when Out is `struct{}` or when the returned
// value is a nil pointer / nil interface (i.e. there's nothing to send).

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
)

// StatusError carries an HTTP status code so handler errors can opt into a
// specific 4xx/5xx response without inventing a new error type per call
// site. Use NewStatusError, or any error that implements `HTTPStatus() int`.
type StatusError struct {
	Code    int
	Message string
	Err     error
}

// NewStatusError builds a StatusError. Message is the public string sent in
// the response body; pass an empty message to fall back to http.StatusText.
func NewStatusError(code int, message string) *StatusError {
	return &StatusError{Code: code, Message: message}
}

// Error implements error.
func (e *StatusError) Error() string {
	switch {
	case e.Message != "":
		return e.Message
	case e.Err != nil:
		return e.Err.Error()
	default:
		return http.StatusText(e.Code)
	}
}

// Unwrap exposes the inner cause for errors.Is / errors.As.
func (e *StatusError) Unwrap() error { return e.Err }

// HTTPStatus is the contract JSONHandler keys off when mapping handler
// errors to response codes.
func (e *StatusError) HTTPStatus() int { return e.Code }

// JSONHandler wraps a typed business function as an http.HandlerFunc. It
// performs bind + validate + invoke + respond in a single step so handlers
// only contain business logic.
//
//	srv.HandleFunc("POST /users", server.JSONHandler(
//	    func(ctx context.Context, in CreateUser) (User, error) {
//	        return createUser(ctx, in)
//	    },
//	))
func JSONHandler[In, Out any](fn func(context.Context, In) (Out, error)) http.HandlerFunc {
	outIsEmpty := isEmptyStructType(reflect.TypeFor[Out]())
	return func(w http.ResponseWriter, r *http.Request) {
		var in In
		if err := BindJSON(r, &in); err != nil {
			writeBindError(w, err)
			return
		}
		out, err := fn(r.Context(), in)
		if err != nil {
			writeHandlerError(w, err)
			return
		}
		if outIsEmpty || isNilValue(out) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// JSONEcho is the shorthand for the validate-and-pass-through case: bind the
// body into T, run validation, and echo the validated value back as the 200
// response. Useful for webhook acks, dev stubs, and "did this payload
// validate?" endpoints where the response shape is the same as the input.
//
//	srv.POST("/users", server.JSONEcho[CreateUser]())
//
// Reach for JSONHandler[In, Out] when the response is genuinely different
// from the input — assigning a server-side ID, lowercasing the email,
// joining a related record. An identity function is the absence of business
// logic; JSONEcho says so directly.
//
// Errors follow JSONHandler: *ValidationError → per-field 400 envelope,
// other bind errors → 400 with {"error": err.Error()}.
func JSONEcho[T any]() http.HandlerFunc {
	return JSONHandler(func(_ context.Context, in T) (T, error) { return in, nil })
}

// fieldErrorPayload is the wire shape for a single validation failure. It
// intentionally omits FieldError.Value so handlers can't accidentally
// leak the offending field (passwords, tokens, …) back to the caller.
type fieldErrorPayload struct {
	Field   string `json:"field"`
	Tag     string `json:"tag,omitempty"`
	Param   string `json:"param,omitempty"`
	Message string `json:"message"`
}

type validationPayload struct {
	Error  string              `json:"error"`
	Fields []fieldErrorPayload `json:"fields"`
}

func writeBindError(w http.ResponseWriter, err error) {
	if verr, ok := errors.AsType[*ValidationError](err); ok {
		payload := validationPayload{
			Error:  "validation failed",
			Fields: make([]fieldErrorPayload, 0, len(verr.Fields)),
		}
		for _, f := range verr.Fields {
			payload.Fields = append(payload.Fields, fieldErrorPayload{
				Field:   f.Field,
				Tag:     f.Tag,
				Param:   f.Param,
				Message: f.Message,
			})
		}
		writeJSON(w, http.StatusBadRequest, payload)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func writeHandlerError(w http.ResponseWriter, err error) {
	if statusErr, ok := errors.AsType[interface {
		error
		HTTPStatus() int
	}](err); ok {
		writeJSON(w, statusErr.HTTPStatus(), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Default().Error("JSONHandler: encode response", "error", err)
	}
}

func isEmptyStructType(t reflect.Type) bool {
	return t != nil && t.Kind() == reflect.Struct && t.NumField() == 0
}

// isNilValue reports whether the dynamic value held by `v` is nil. Needed
// because `any(nilPtr) == nil` is false: the interface is non-nil with a
// nil concrete value.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Slice:
		return rv.IsNil()
	}
	return false
}
