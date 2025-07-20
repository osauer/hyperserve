# HyperServe Authentication Example

This example demonstrates a production-ready authentication system using HyperServe with multiple authentication methods, security best practices, and environment-specific configurations.

## Features

### Authentication Methods
- **JWT (RS256)**: Industry-standard token-based authentication with RSA signature verification
- **API Keys**: For service-to-service communication with per-key rate limiting
- **Basic Auth**: Development/emergency access (disabled in production)

### Security Features
- **Rate Limiting**: Per-token rate limiting to prevent brute force attacks
- **Timing-Safe Validation**: Prevents timing attacks using `crypto/subtle`
- **Audit Logging**: Comprehensive logging of all authentication events
- **Security Headers**: CORS, CSP, HSTS, and other security headers
- **Environment-Specific Config**: Separate configurations for dev/staging/prod

### RBAC Support
- Role-based access control
- Fine-grained permissions
- Middleware for permission checking

## Quick Start

### Development Mode

```bash
# Run with default development settings
go run main.go

# Or with environment variable
APP_ENV=development go run main.go
```

### Test Authentication Methods

1. **API Key Authentication**:
```bash
# Admin access
curl -H 'Authorization: APIKey dev_api_key_admin' http://localhost:8080/api/profile

# User access
curl -H 'Authorization: APIKey dev_api_key_user' http://localhost:8080/api/profile
```

2. **Basic Authentication**:
```bash
# Using curl's -u flag
curl -u admin:admin123 http://localhost:8080/api/profile

# Or with Authorization header
curl -H 'Authorization: Basic YWRtaW46YWRtaW4xMjM=' http://localhost:8080/api/profile
```

3. **JWT Authentication**:
```bash
# Get a JWT token
curl -X POST http://localhost:8080/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"testuser","password":"testpass"}'

# Use the token
curl -H 'Authorization: Bearer <your-jwt-token>' http://localhost:8080/api/profile
```

## Production Setup

### 1. Generate RSA Keys for JWT

```bash
# Generate private key
openssl genrsa -out jwt-private.pem 2048

# Extract public key
openssl rsa -in jwt-private.pem -pubout -out jwt-public.pem
```

### 2. Environment Variables

```bash
export APP_ENV=production
export JWT_PUBLIC_KEY=$(cat jwt-public.pem)
export DATABASE_URL="postgres://user:pass@localhost/authdb"
```

### 3. Run in Production

```bash
APP_ENV=production ./auth-example
```

## Architecture

### Multi-Provider Authentication

The example uses a provider-based architecture that allows easy extension:

```go
type AuthProvider interface {
    Validate(token string) (SessionInfo, error)
    Name() string
}
```

### Session Information

Each successful authentication returns rich session information:

```go
type SessionInfo struct {
    UserID      string
    Username    string
    Roles       []string
    Permissions []string
    ExpiresAt   time.Time
    Metadata    map[string]string
}
```

### Audit Logging

All authentication attempts are logged with context:
- Success: User ID, username, auth method, metadata
- Failure: Reason, partial token (for debugging), metadata

## Security Considerations

### Development vs Production

- **Development**: All auth methods enabled, relaxed security for testing
- **Production**: Only JWT and API keys, strict security headers, TLS required

### Token Storage

- Never log full tokens
- Use secure key management (AWS KMS, HashiCorp Vault, etc.)
- Rotate keys regularly

### Rate Limiting

- Per-token rate limiting (10 requests/minute by default)
- Configurable per API key
- Global rate limiting via HyperServe middleware

## Extending the Example

### Add OAuth2/OIDC

```go
type OAuth2Provider struct {
    clientID     string
    clientSecret string
    redirectURL  string
}

func (o *OAuth2Provider) Validate(token string) (SessionInfo, error) {
    // Validate with OAuth2 provider
}
```

### Add Database Backend

```go
type DatabaseAPIKeyProvider struct {
    db *sql.DB
}

func (d *DatabaseAPIKeyProvider) Validate(token string) (SessionInfo, error) {
    // Look up API key in database
}
```

### Add JWT Refresh Tokens

```go
srv.POST("/refresh", func(w http.ResponseWriter, r *http.Request) {
    // Validate refresh token
    // Issue new access token
})
```

## Testing

```bash
# Run tests
go test -v

# Test with coverage
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Benchmarks

```bash
# Run benchmarks
go test -bench=. -benchmem

# Profile CPU usage
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof
```

## Monitoring

The example includes metrics for:
- Authentication success/failure rates
- Provider-specific metrics
- Rate limit hits
- Response times

## Troubleshooting

### Common Issues

1. **"Invalid token format"**: Ensure you're using the correct Authorization header format
2. **"Rate limit exceeded"**: Wait 6 seconds between requests or increase limits
3. **"Invalid credentials"**: Check token/key validity and expiration

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug go run main.go
```

## License

See the main HyperServe LICENSE file.