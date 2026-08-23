module github.com/osauer/hyperserve/examples/auth

go 1.27

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/osauer/hyperserve v0.0.0
	golang.org/x/time v0.15.0
)

replace github.com/osauer/hyperserve => ../../
