#!/bin/bash

# Generate self-signed certificates for testing
# DO NOT use these in production!

echo "Generating self-signed certificates for testing..."

# Generate private key
openssl genpkey -algorithm RSA -out key.pem -pkeyopt rsa_keygen_bits:2048

# Generate certificate
openssl req -new -x509 -key key.pem -out cert.pem -days 365 \
    -subj "/C=US/ST=State/L=City/O=Enterprise/CN=localhost"

echo "Certificates generated:"
echo "  - cert.pem (certificate)"
echo "  - key.pem (private key)"
echo ""
echo "WARNING: These are self-signed certificates for testing only!"
echo "For production, use certificates from a trusted CA."