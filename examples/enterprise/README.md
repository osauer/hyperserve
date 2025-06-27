# Enterprise Security Example

This example demonstrates HyperServe's enterprise-grade security features available in Go 1.24.

## Features Demonstrated

- **FIPS 140-3 Compliance**: Government-grade cryptographic standards
- **Encrypted Client Hello (ECH)**: Privacy-enhanced TLS
- **Post-Quantum Cryptography**: Future-proof key exchange
- **Timing-Safe Authentication**: Protection against timing attacks
- **Secure File Serving**: Using os.Root for sandboxed access
- **Swiss Tables Rate Limiting**: High-performance request limiting

## Quick Start

1. Generate test certificates:
   ```bash
   ./generate_certs.sh
   ```

2. Create directories:
   ```bash
   mkdir -p templates static
   echo "<h1>Secure Static File</h1>" > static/index.html
   ```

3. Run the server:
   ```bash
   go run main.go
   ```

4. Test the endpoints:
   ```bash
   # Public endpoint
   curl -k https://localhost:8443/health

   # Protected endpoint (will return 401)
   curl -k https://localhost:8443/api/status

   # Protected endpoint with authentication
   curl -k -H "Authorization: Bearer enterprise-key-123" https://localhost:8443/api/status
   ```

## Security Configuration

### FIPS 140-3 Mode

When enabled, the server:
- Uses only FIPS-approved cipher suites (AES-GCM)
- Restricts to P256 and P384 elliptic curves
- Enables GOFIPS140 runtime mode
- Logs compliance status

### Encrypted Client Hello

ECH protects the Server Name Indication (SNI) in TLS handshakes:
- Prevents eavesdropping on which server is being accessed
- Requires ECH-capable clients
- Falls back gracefully for non-ECH clients

### Post-Quantum Security

When not in FIPS mode:
- Automatically enables X25519MLKEM768 key exchange
- Provides protection against future quantum computers
- Maintains compatibility with current clients

## Production Considerations

1. **Certificates**: Use real certificates from a trusted CA
2. **ECH Keys**: Generate and rotate ECH keys securely
3. **Token Validation**: Implement proper JWT or database-backed validation
4. **Monitoring**: Enable metrics and logging for security events
5. **Compliance**: Verify FIPS mode meets your regulatory requirements

## Testing TLS Configuration

Check your TLS configuration:
```bash
# View server certificate
openssl s_client -connect localhost:8443 -showcerts

# Check cipher suites (FIPS mode)
nmap --script ssl-enum-ciphers -p 8443 localhost
```

## Environment Variables

- `GOFIPS140=1`: Enable FIPS 140-3 runtime mode
- `HS_LOG_LEVEL=debug`: Enable debug logging

## Troubleshooting

1. **Certificate errors**: The example uses self-signed certificates. Use `-k` with curl.
2. **FIPS mode errors**: Ensure Go 1.24 is properly installed
3. **ECH not working**: Check client support for Encrypted Client Hello