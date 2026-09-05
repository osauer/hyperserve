module github.com/osauer/hyperserve/examples/auth

go 1.27

require (
	github.com/coreos/go-oidc/v3 v3.20.0
	github.com/osauer/hyperserve/v2 v2.1.2
)

require (
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace github.com/osauer/hyperserve/v2 => ../../
