module github.com/osauer/hyperserve/v2/tools

go 1.27.3

require (
	github.com/google/jsonschema-go v0.4.3
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/osauer/hyperserve/v2 v2.2.2
)

require (
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	golang.org/x/tools/gopls v0.23.0 // indirect
)

replace github.com/osauer/hyperserve/v2 => ..

tool golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize
