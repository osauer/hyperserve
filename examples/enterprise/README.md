# Restricted TLS policy

This advanced example enables TLS and asks HyperServe to restrict the handshake
to the cipher suites and elliptic curves selected by `WithFIPSMode`.

The option is deliberately narrower than its historical name may suggest. It
does not select a FIPS-validated Go toolchain, constrain cryptography outside
TLS, or establish FIPS 140-3 compliance. It also does not enable Encrypted
Client Hello or post-quantum key exchange.

## Run

Create a short-lived development certificate, then start the example from this
directory:

```sh
cd examples/enterprise
openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 1 \
  -subj '/CN=localhost' -keyout key.pem -out cert.pem
go run .
```

In another terminal:

```sh
curl -k https://localhost:8443/
curl http://localhost:9080/healthz/
```

The first command uses `-k` only because the development certificate is
self-signed. Production certificates still require normal hostname and trust
validation.

## What the option changes

`WithFIPSMode` restricts the TLS configuration to AES-GCM cipher suites and the
P-256/P-384 curves. HyperServe logs the same narrow boundary when the server
starts. See the option's Go documentation before using it as one part of a
regulated deployment.
