package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type tUserIn struct {
	Name  string `json:"name" validate:"required,min=2"`
	Email string `json:"email" validate:"required,email"`
}

type tUserOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestJSONHandler(t *testing.T) {
	t.Parallel()

	notFound := NewStatusError(http.StatusNotFound, "user not found")
	boom := errors.New("kaboom")

	okHandler := JSONHandler(func(_ context.Context, in tUserIn) (tUserOut, error) {
		return tUserOut{ID: "u1", Name: in.Name}, nil
	})
	statusHandler := JSONHandler(func(_ context.Context, _ tUserIn) (tUserOut, error) {
		return tUserOut{}, notFound
	})
	genericErrHandler := JSONHandler(func(_ context.Context, _ tUserIn) (tUserOut, error) {
		return tUserOut{}, boom
	})
	emptyOutHandler := JSONHandler(func(_ context.Context, _ tUserIn) (struct{}, error) {
		return struct{}{}, nil
	})
	nilPtrHandler := JSONHandler(func(_ context.Context, _ tUserIn) (*tUserOut, error) {
		return nil, nil
	})

	cases := []struct {
		name       string
		handler    http.HandlerFunc
		body       string
		wantStatus int
		// wantContains is a list of substrings the response body must include.
		wantContains []string
		// wantNoBody, when true, asserts the response body is empty (204).
		wantNoBody bool
	}{
		{
			name:         "success returns 200 + JSON-encoded Out",
			handler:      okHandler,
			body:         `{"name":"Ada","email":"ada@example.com"}`,
			wantStatus:   http.StatusOK,
			wantContains: []string{`"id":"u1"`, `"name":"Ada"`},
		},
		{
			name:         "validation failure returns 400 with per-field envelope",
			handler:      okHandler,
			body:         `{"name":"A","email":"not-an-email"}`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"error":"validation failed"`, `"field":"name"`, `"tag":"min"`, `"field":"email"`},
		},
		{
			name:         "bind failure (malformed JSON) returns 400 with plain envelope",
			handler:      okHandler,
			body:         `{"name":`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"error":"BindJSON:`},
		},
		{
			name:         "bind failure (unknown field) returns 400",
			handler:      okHandler,
			body:         `{"name":"Ada","email":"ada@x.com","oops":1}`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"error":"BindJSON:`, "oops"},
		},
		{
			name:         "handler returns StatusError → its status + message",
			handler:      statusHandler,
			body:         `{"name":"Ada","email":"ada@x.com"}`,
			wantStatus:   http.StatusNotFound,
			wantContains: []string{`"error":"user not found"`},
		},
		{
			name:         "handler returns plain error → 500 with generic message (no leak)",
			handler:      genericErrHandler,
			body:         `{"name":"Ada","email":"ada@x.com"}`,
			wantStatus:   http.StatusInternalServerError,
			wantContains: []string{`"error":"internal server error"`},
		},
		{
			name:       "Out=struct{} returns 204 with empty body",
			handler:    emptyOutHandler,
			body:       `{"name":"Ada","email":"ada@x.com"}`,
			wantStatus: http.StatusNoContent,
			wantNoBody: true,
		},
		{
			name:       "Out=*T with nil return returns 204",
			handler:    nilPtrHandler,
			body:       `{"name":"Ada","email":"ada@x.com"}`,
			wantStatus: http.StatusNoContent,
			wantNoBody: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()
			if res.StatusCode != tc.wantStatus {
				t.Fatalf("status: want %d, got %d (body=%s)", tc.wantStatus, res.StatusCode, w.Body.String())
			}
			body := w.Body.String()
			if tc.wantNoBody {
				if body != "" {
					t.Fatalf("expected empty body, got %q", body)
				}
				return
			}
			if got := res.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type: want application/json, got %q", got)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(body, want) {
					t.Fatalf("body missing %q\nfull body: %s", want, body)
				}
			}
		})
	}
}

// TestJSONHandler_ValidationEnvelopeShape locks the JSON shape so future
// changes to fieldErrorPayload break this test instead of silently breaking
// API clients.
func TestJSONHandler_ValidationEnvelopeShape(t *testing.T) {
	t.Parallel()
	h := JSONHandler(func(_ context.Context, _ tUserIn) (tUserOut, error) {
		return tUserOut{}, nil
	})
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"","email":""}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, r)

	var got struct {
		Error  string `json:"error"`
		Fields []struct {
			Field   string `json:"field"`
			Tag     string `json:"tag"`
			Param   string `json:"param"`
			Message string `json:"message"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%s", err, w.Body.String())
	}
	if got.Error != "validation failed" {
		t.Fatalf("error: want %q, got %q", "validation failed", got.Error)
	}
	if len(got.Fields) == 0 {
		t.Fatalf("expected at least one field error, got none")
	}
	for _, f := range got.Fields {
		if f.Field == "" || f.Message == "" || f.Tag == "" {
			t.Fatalf("field entry missing required keys: %+v", f)
		}
	}
}

// TestJSONEcho covers the two behaviours that distinguish JSONEcho from a
// plain JSONHandler: a successful request echoes the validated input
// verbatim, and a validation failure emits the same per-field 400 envelope
// JSONHandler renders. Other error paths are exercised by TestJSONHandler
// since JSONEcho is a thin wrapper over it.
func TestJSONEcho(t *testing.T) {
	t.Parallel()
	h := JSONEcho[tUserIn]()

	t.Run("success echoes the validated input verbatim", func(t *testing.T) {
		t.Parallel()
		body := `{"name":"Ada","email":"ada@example.com"}`
		r := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h(w, r)

		res := w.Result()
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status: want 200, got %d (body=%s)", res.StatusCode, w.Body.String())
		}
		if got := res.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type: want application/json, got %q", got)
		}
		var got tUserIn
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode echo body: %v\nbody=%s", err, w.Body.String())
		}
		want := tUserIn{Name: "Ada", Email: "ada@example.com"}
		if got != want {
			t.Fatalf("echo body: want %+v, got %+v", want, got)
		}
	})

	t.Run("validation failure emits per-field 400 envelope", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"name":"A","email":"not-an-email"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h(w, r)

		res := w.Result()
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status: want 400, got %d (body=%s)", res.StatusCode, w.Body.String())
		}
		body := w.Body.String()
		for _, want := range []string{`"error":"validation failed"`, `"field":"name"`, `"tag":"min"`, `"field":"email"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("body missing %q\nfull body: %s", want, body)
			}
		}
	})
}

func TestStatusError(t *testing.T) {
	t.Parallel()
	inner := errors.New("db gone")
	cases := []struct {
		name      string
		err       *StatusError
		wantMsg   string
		wantCode  int
		wantUnwra error
	}{
		{
			name:      "explicit message wins",
			err:       &StatusError{Code: 418, Message: "teapot", Err: inner},
			wantMsg:   "teapot",
			wantCode:  418,
			wantUnwra: inner,
		},
		{
			name:      "falls back to wrapped error",
			err:       &StatusError{Code: 502, Err: inner},
			wantMsg:   "db gone",
			wantCode:  502,
			wantUnwra: inner,
		},
		{
			name:     "falls back to status text",
			err:      NewStatusError(http.StatusConflict, ""),
			wantMsg:  http.StatusText(http.StatusConflict),
			wantCode: http.StatusConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.wantMsg {
				t.Fatalf("Error(): want %q, got %q", tc.wantMsg, got)
			}
			if got := tc.err.HTTPStatus(); got != tc.wantCode {
				t.Fatalf("HTTPStatus(): want %d, got %d", tc.wantCode, got)
			}
			if !errors.Is(tc.err, tc.wantUnwra) && tc.wantUnwra != nil {
				t.Fatalf("Unwrap chain missing %v", tc.wantUnwra)
			}
		})
	}
}
