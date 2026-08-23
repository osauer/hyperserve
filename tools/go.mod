module github.com/osauer/hyperserve/tools

go 1.27

require github.com/osauer/hyperserve v0.0.0

require (
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.47.1-0.20260707181000-a299dadba899 // indirect
	golang.org/x/tools/gopls v0.23.0 // indirect
)

replace github.com/osauer/hyperserve => ..

tool golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize
