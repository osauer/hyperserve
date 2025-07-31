module github.com/osauer/hyperserve/examples/auth

go 1.24

toolchain go1.24.4

require (
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/osauer/hyperserve v0.0.0
	golang.org/x/time v0.7.0
)

replace github.com/osauer/hyperserve => ../../
