package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/osauer/hyperserve/v2/auth"
)

type fakeIDTokenVerifier struct {
	token *oidc.IDToken
	err   error
}

func (v fakeIDTokenVerifier) Verify(context.Context, string) (*oidc.IDToken, error) {
	return v.token, v.err
}

func TestOIDCVerifierProducesStablePrincipal(t *testing.T) {
	verifier := oidcVerifier{tokens: fakeIDTokenVerifier{token: &oidc.IDToken{
		Issuer:  "https://accounts.example",
		Subject: "user-123",
	}}}

	principal, err := verifier.VerifyToken(context.Background(), "token")
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if principal.Issuer != "https://accounts.example" || principal.Subject != "user-123" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestOIDCVerifierClassifiesRejectedCredential(t *testing.T) {
	verifier := oidcVerifier{tokens: fakeIDTokenVerifier{err: errors.New("signature rejected")}}
	_, err := verifier.VerifyToken(context.Background(), "bad-token")
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("error = %v, want auth.ErrUnauthenticated", err)
	}
}

type staticTokenVerifier struct{}

func (staticTokenVerifier) VerifyToken(context.Context, string) (auth.Principal, error) {
	return auth.Principal{Issuer: "https://accounts.example", Subject: "user-123"}, nil
}

func TestProtectedRouteReceivesPrincipal(t *testing.T) {
	app, err := newApp("127.0.0.1:0", staticTokenVerifier{})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, "subject=user-123") {
		t.Fatalf("body = %q", body)
	}
}
