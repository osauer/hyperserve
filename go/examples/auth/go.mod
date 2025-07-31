module github.com/osauer/hyperserve/examples/auth

go 1.24

toolchain go1.24.4

require (
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/osauer/hyperserve v0.0.0
	golang.org/x/time v0.7.0
)

replace github.com/osauer/hyperserve => ../../
