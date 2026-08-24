// Package auth provides the small authentication boundary needed by HTTP
// applications without owning identity-provider setup, sessions, or
// application authorization.
package auth

import (
	"context"
	"errors"
	"net/http"
)

// ErrUnauthenticated reports absent, invalid, or expired credentials. An
// Authenticator should wrap this error when the client may safely retry with
// different credentials. Other errors are treated as internal failures.
var ErrUnauthenticated = errors.New("unauthenticated")

// Principal identifies one authenticated subject. Issuer and Subject form the
// stable identity key used by OpenID Connect and prevent subject collisions
// between identity providers.
type Principal struct {
	Issuer  string
	Subject string
}

// Valid reports whether the principal has a complete stable identity.
func (p Principal) Valid() bool {
	return p.Issuer != "" && p.Subject != ""
}

// Authenticator establishes the principal for an HTTP request.
type Authenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

// AuthenticatorFunc adapts a named function to Authenticator, following the
// same interface-plus-function pattern as http.Handler and http.HandlerFunc.
type AuthenticatorFunc func(*http.Request) (Principal, error)

// Authenticate calls f(r).
func (f AuthenticatorFunc) Authenticate(r *http.Request) (Principal, error) {
	return f(r)
}

type principalContextKey struct{}

type challenger interface {
	Challenge() string
}

// PrincipalFromContext returns the principal established by Require.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok && principal.Valid()
}

// PrincipalFromRequest returns the principal established for r.
func PrincipalFromRequest(r *http.Request) (Principal, bool) {
	if r == nil {
		return Principal{}, false
	}
	return PrincipalFromContext(r.Context())
}

// Require authenticates each request and places its Principal in the request
// context. Authentication failures receive 401; provider or configuration
// failures receive a generic 500 without leaking details.
func Require(authenticator Authenticator) func(http.Handler) http.Handler {
	if authenticator == nil {
		panic("auth: nil Authenticator")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, err := authenticator.Authenticate(r)
			if err != nil {
				if errors.Is(err, ErrUnauthenticated) {
					if source, ok := authenticator.(challenger); ok {
						w.Header().Set("WWW-Authenticate", source.Challenge())
					}
					http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
					return
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if !principal.Valid() {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
