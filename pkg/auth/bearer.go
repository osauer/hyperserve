package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// TokenVerifier validates a bearer token and returns its stable principal.
// OIDC and other federated adapters can implement this interface without
// coupling provider-specific claims to the server package.
type TokenVerifier interface {
	VerifyToken(context.Context, string) (Principal, error)
}

// TokenVerifierFunc adapts a named function to TokenVerifier.
type TokenVerifierFunc func(context.Context, string) (Principal, error)

// VerifyToken calls f(ctx, token).
func (f TokenVerifierFunc) VerifyToken(ctx context.Context, token string) (Principal, error) {
	return f(ctx, token)
}

type bearerAuthenticator struct {
	verifier TokenVerifier
}

// Bearer builds an Authenticator for RFC 6750 Authorization headers.
func Bearer(verifier TokenVerifier) Authenticator {
	if verifier == nil {
		panic("auth: nil TokenVerifier")
	}
	return bearerAuthenticator{verifier: verifier}
}

func (a bearerAuthenticator) Authenticate(r *http.Request) (Principal, error) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return Principal{}, ErrUnauthenticated
	}
	principal, err := a.verifier.VerifyToken(r.Context(), token)
	if err != nil {
		return Principal{}, err
	}
	if !principal.Valid() {
		return Principal{}, fmt.Errorf("auth: verifier returned an incomplete principal")
	}
	return principal, nil
}

func (bearerAuthenticator) Challenge() string {
	return "Bearer"
}
