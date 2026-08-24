package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireStoresPrincipal(t *testing.T) {
	want := Principal{Issuer: "https://issuer.example", Subject: "user-123"}
	authenticator := AuthenticatorFunc(func(*http.Request) (Principal, error) {
		return want, nil
	})
	handler := Require(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := PrincipalFromRequest(r)
		if !ok || got != want {
			t.Fatalf("principal = %#v, %v; want %#v, true", got, ok, want)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRequireErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "invalid credentials", err: errors.Join(ErrUnauthenticated, errors.New("expired")), want: http.StatusUnauthorized},
		{name: "provider failure", err: errors.New("issuer unavailable"), want: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := Require(AuthenticatorFunc(func(*http.Request) (Principal, error) {
				return Principal{}, test.err
			}))(http.NotFoundHandler())
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestBearer(t *testing.T) {
	verifier := TokenVerifierFunc(func(_ context.Context, token string) (Principal, error) {
		if token != "good-token" {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{Issuer: "local", Subject: "operator"}, nil
	})
	handler := Require(Bearer(verifier))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, test := range []struct {
		header string
		want   int
	}{
		{header: "Bearer good-token", want: http.StatusNoContent},
		{header: "bearer good-token", want: http.StatusNoContent},
		{header: "Bearer bad-token", want: http.StatusUnauthorized},
		{header: "", want: http.StatusUnauthorized},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", test.header)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Errorf("header %q: status = %d, want %d", test.header, recorder.Code, test.want)
		}
		if test.want == http.StatusUnauthorized && recorder.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("header %q: WWW-Authenticate = %q, want Bearer", test.header, recorder.Header().Get("WWW-Authenticate"))
		}
	}
}
