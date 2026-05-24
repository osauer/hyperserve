# Version stamping. `git describe` picks the nearest tag; --dirty marks
# uncommitted working trees so dev builds are distinguishable.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%d_%H:%M:%S_UTC" || echo "unknown")

# Stamped into pkg/server.Version/BuildHash/BuildTime via -X.
LDFLAGS := -ldflags "-X github.com/osauer/hyperserve/pkg/server.Version=$(VERSION) -X github.com/osauer/hyperserve/pkg/server.BuildHash=$(BUILD_HASH) -X github.com/osauer/hyperserve/pkg/server.BuildTime=$(BUILD_TIME)"

.PHONY: build install test test-race fuzz-smoke clean version check check-examples check-canonical-examples vet fmt modernize modernize-check staticcheck govulncheck

build: ## Compile hyperserve-init with version stamped via ldflags
	mkdir -p bin
	go build $(LDFLAGS) -o bin/hyperserve-init ./cmd/hyperserve-init

install: ## Install hyperserve-init via `go install`
	go install $(LDFLAGS) ./cmd/hyperserve-init

# `test` keeps the historical "check + verbose tests" shape. CI should call
# test-race additionally — race detection is slow enough that we don't make
# it the default for `make test`.
test: check ## Run the check gate, then `go test -v ./...`
	go test -v ./...

test-race: ## Run the full test suite under the race detector.
	go test -race ./...

# fuzz-smoke runs every fuzz target for a short, fixed budget so CI catches
# panics in the parsers without committing to a long fuzz run. Tune the
# duration upward (5m / 30m / hour) in nightly jobs.
#
# `set -e` + no `|| true` = a fuzzing failure on any target fails the whole
# target. The earlier shape used `|| true` and was incapable of gating; this
# one will surface a discovered crash as a non-zero make exit.
fuzz-smoke: ## Short fuzz pass over every Fuzz* target (15s each).
	@set -e; \
	go test -run=^$$ -fuzz=FuzzJSONRPCParse        -fuzztime=15s ./pkg/jsonrpc; \
	go test -run=^$$ -fuzz=FuzzWebSocketFrameParse -fuzztime=15s ./pkg/websocket; \
	go test -run=^$$ -fuzz=FuzzCORSOriginMatch     -fuzztime=15s ./pkg/server; \
	go test -run=^$$ -fuzz=FuzzValidateEmail       -fuzztime=15s ./pkg/server

clean:
	rm -rf bin hyperserve

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
check: vet staticcheck govulncheck modernize-check check-examples check-canonical-examples ## gofmt + vet + staticcheck + govulncheck + modernize-check + example gates
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
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not on PATH; install: go install honnef.co/go/tools/cmd/staticcheck@v0.7.0" >&2; exit 1; }
	staticcheck ./...

govulncheck:
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck not on PATH; install: go install golang.org/x/vuln/cmd/govulncheck@v1.3.0" >&2; exit 1; }
	govulncheck ./...

# Standalone example modules (own go.mod via `replace`) live outside the
# main module's `./...`, so vet + govulncheck against the root never sees
# them. Dependabot caught a HIGH vuln in examples/auth/go.mod
# (golang-jwt v5.2.1, GHSA-mh63-6h87-95cp) that the root govulncheck had
# missed for exactly this reason — closing the process gap here.
#
# Discovery is by shell glob, so new standalone examples are picked up
# automatically without Makefile edits.
EXAMPLE_MODULES := $(shell ls examples/*/go.mod 2>/dev/null | xargs -n1 dirname)

check-examples: ## go vet + build + govulncheck in each standalone examples/*/ module
	@if [ -z "$(EXAMPLE_MODULES)" ]; then \
		echo "no standalone example modules found"; \
		exit 0; \
	fi
	@for mod in $(EXAMPLE_MODULES); do \
		echo "--- $$mod"; \
		(cd $$mod && go vet ./... && go build ./... && govulncheck ./...) || exit 1; \
	done

check-canonical-examples: ## Build the release-gated MCP, SSE, and API examples
	go test ./examples/devops ./examples/mcp-sse ./examples/json-api

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
