# Federated authentication

This example connects HyperServe's small authentication boundary to an OpenID
Connect provider. The provider library discovers signing keys and validates the
token's signature, issuer, audience, and expiry. HyperServe receives only the
stable identity pair: issuer plus subject.

The composition in `newServer` is intentionally named instead of nested:

```go
bearerIdentity := auth.Bearer(verifier)
requireIdentity := auth.Require(bearerIdentity)
srv.UsePrefix("/api", requireIdentity)
```

That reads as three separate decisions: how credentials arrive, whether an
identity is required, and which routes require it. Handlers retrieve the result
with `auth.PrincipalFromRequest`.

## Run it

Register an OIDC client with your provider, then set its issuer and audience:

```sh
export OIDC_ISSUER=https://accounts.example.com
export OIDC_CLIENT_ID=my-api-client
go run .
```

Call the protected endpoint with a token issued for that client:

```sh
curl -H "Authorization: Bearer $ID_TOKEN" http://localhost:8090/api/me
```

## Important boundary

This example verifies an OIDC ID token whose audience is `OIDC_CLIENT_ID`.
Many providers issue a different access-token format for APIs, sometimes an
opaque token. Follow your provider's resource-server guidance in that case and
implement the same `auth.TokenVerifier` interface with its SDK or introspection
endpoint. Do not parse an unverified JWT or assume every access token is an ID
token.

HyperServe does not implement the browser redirect, authorization-code
exchange, cookies, refresh tokens, logout, or application permissions. Those
policies differ by application and provider. The library establishes a stable
principal; your handler or authorization middleware decides what that
principal may do.
