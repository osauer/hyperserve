# Version stamping. `git describe` picks the nearest tag; --dirty marks
# uncommitted working trees so dev builds are distinguishable.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%d_%H:%M:%S_UTC" || echo "unknown")

# Stamped into pkg/server.Version/BuildHash/BuildTime via -X.
LDFLAGS := -ldflags "-X github.com/osauer/hyperserve/pkg/server.Version=$(VERSION) -X github.com/osauer/hyperserve/pkg/server.BuildHash=$(BUILD_HASH) -X github.com/osauer/hyperserve/pkg/server.BuildTime=$(BUILD_TIME)"

.PHONY: build install test clean version check vet fmt modernize modernize-check staticcheck govulncheck

build: ## Compile cmd/server with version stamped via ldflags
	go build $(LDFLAGS) -o hyperserve ./cmd/server

install: ## Install hyperserve via `go install`
	go install $(LDFLAGS) ./cmd/server

test: ## Run go test -v ./...
	go test -v ./...

clean:
	rm -f hyperserve

version: ## Print the version string the next build would embed
	@echo $(VERSION)

# Binding pre-commit gate: gofmt drift + go vet + staticcheck + govulncheck +
# go-fix/modernize drift. Mirrors the pattern in ../ibkr.
#
# Why staticcheck and govulncheck use `command -v` (not lazy install): these
# are developer-machine tools, expected to be installed once. The Makefile
# tells you the exact command if missing. Modernize is different — it's
# pinned via the `tool` directive in go.mod and invoked via `go tool`, so it
# auto-downloads on first use and stays reproducible across machines/CI.
check: vet staticcheck govulncheck modernize-check ## gofmt + vet + staticcheck + govulncheck + modernize-check
	@# gofmt over tracked + untracked-but-not-gitignored .go files. Same
	@# pattern as ibkr — `git ls-files` respects .gitignore so this skips
	@# /dist, agent worktrees, etc. The intermediate exists-check filters
	@# files git knows about but that don't exist on disk (staged-for-delete
	@# mid-commit), otherwise gofmt prints lstat errors for each one.
	@unformatted=$$( \
		git ls-files --cached --others --exclude-standard '*.go' | \
		while IFS= read -r f; do [ -e "$$f" ] && printf '%s\n' "$$f"; done | \
		xargs gofmt -l \
	); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: the following files need formatting:"; \
		echo "$$unformatted"; \
		echo "fix with: make fmt"; \
		exit 1; \
	fi

vet:
	go vet ./...

staticcheck:
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not on PATH; install: go install honnef.co/go/tools/cmd/staticcheck@latest" >&2; exit 1; }
	staticcheck ./...

govulncheck:
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck not on PATH; install: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2; exit 1; }
	govulncheck ./...

# Idiom-drift gate. `go fix -diff` is the toolchain-native fixer (tracks the
# Go version pinned in go.mod); `go tool modernize` runs the broader gopls
# analyzer suite (range N, wg.Go, b.Loop, maps.Copy, SplitSeq, any vs
# interface{}, etc.). Version of modernize is pinned via the `tool` directive
# in go.mod, so the gate is reproducible without an `@latest` install step.
#
# Stream discipline + chatter filter:
#   - `go fix -diff` writes the unified diff to stdout, download chatter to
#     stderr → capture stdout (no redirect needed; stderr stays visible).
#   - `go tool modernize` writes diagnostics AND `go: downloading …` lines to
#     stderr (the latter when go.mod's tool deps aren't cached — every fresh
#     CI run hits this). Same stream means we can't separate by redirection;
#     instead we capture stderr via stream-swap and grep the chatter out.
modernize-check: ## go fix -diff + modernize gate (Go idiom drift vs go.mod's go version)
	@out=$$(go fix -diff ./...); \
	if [ -n "$$out" ]; then \
		echo "go fix found pending changes:"; echo "$$out"; \
		echo "apply with: make modernize"; exit 1; \
	fi
	@out=$$(go tool modernize ./... 2>&1 1>/dev/null | grep -v '^go: downloading'); \
	if [ -n "$$out" ]; then \
		echo "modernize found pending changes:"; echo "$$out"; \
		echo "apply with: make modernize"; exit 1; \
	fi

modernize: ## Apply go fix + modernize rewrites in place
	go fix ./...
	go tool modernize -fix ./...

fmt: ## gofmt -w over tracked / non-gitignored .go files (same scope as `make check`)
	@git ls-files --cached --others --exclude-standard '*.go' | \
		while IFS= read -r f; do [ -e "$$f" ] && printf '%s\n' "$$f"; done | \
		xargs gofmt -w
