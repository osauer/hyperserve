// Command auth shows how a HyperServe API can accept identities from an
// OpenID Connect provider without coupling the server package to that provider.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/osauer/hyperserve/v2/pkg/auth"
	"github.com/osauer/hyperserve/v2/pkg/server"
)

type idTokenVerifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}

// oidcVerifier is the provider-specific adapter. The rest of the application
// only depends on auth.TokenVerifier.
type oidcVerifier struct {
	tokens idTokenVerifier
}

func (v oidcVerifier) VerifyToken(ctx context.Context, rawToken string) (auth.Principal, error) {
	token, err := v.tokens.Verify(ctx, rawToken)
	if err != nil {
		// Mark credential failures explicitly. auth.Require will return 401
		// without exposing the provider's detailed error to the client.
		return auth.Principal{}, fmt.Errorf("%w: verify OIDC token: %v", auth.ErrUnauthenticated, err)
	}
	return auth.Principal{Issuer: token.Issuer, Subject: token.Subject}, nil
}

func newServer(addr string, verifier auth.TokenVerifier) (*server.Server, error) {
	srv, err := server.NewServer(server.WithAddr(addr))
	if err != nil {
		return nil, err
	}

	// Naming the two composition steps makes the policy read from left to
	// right: verify a bearer token, then require that identity under /api.
	bearerIdentity := auth.Bearer(verifier)
	requireIdentity := auth.Require(bearerIdentity)
	srv.UsePrefix("/api", requireIdentity)

	srv.GET("/api/me", handleMe)
	return srv, nil
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromRequest(r)
	if !ok {
		http.Error(w, "missing authenticated principal", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "issuer=%s\nsubject=%s\n", principal.Issuer, principal.Subject)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	issuer := os.Getenv("OIDC_ISSUER")
	clientID := os.Getenv("OIDC_CLIENT_ID")
	if issuer == "" || clientID == "" {
		log.Fatal("OIDC_ISSUER and OIDC_CLIENT_ID are required")
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		log.Fatalf("discover OIDC provider: %v", err)
	}
	verifier := oidcVerifier{
		tokens: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}

	srv, err := newServer(":8090", verifier)
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
