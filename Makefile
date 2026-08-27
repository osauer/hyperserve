# Version stamping. `git describe` picks the nearest tag; --dirty marks
# uncommitted working trees so dev builds are distinguishable.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%d_%H:%M:%S_UTC" || echo "unknown")

# Stamped into the root package's Version/BuildHash/BuildTime via -X.
LDFLAGS := -ldflags "-X github.com/osauer/hyperserve/v2.Version=$(VERSION) -X github.com/osauer/hyperserve/v2.BuildHash=$(BUILD_HASH) -X github.com/osauer/hyperserve/v2.BuildTime=$(BUILD_TIME)"

MAIN_BRANCH ?= main
RELEASE_TEST_JOBS ?= 2

.PHONY: build install test test-race fuzz-smoke benchmark-load clean version help check check-docs check-examples check-canonical-examples check-compatibility-examples mcp-conformance vet fmt modernize modernize-check staticcheck govulncheck govulncheck-tools changelog-lint changelog-stub release-notes release-ci release-gate-test release-publish release-smoke release

help: ## List available targets
	@awk 'BEGIN {FS = ":.*##"; print "Available targets:\n"} \
		/^[a-zA-Z][a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2 }' \
		$(MAKEFILE_LIST)
	@echo
	@echo "Common flow:  make fmt && make check && make test-race && make fuzz-smoke"
	@echo "Release flow: make changelog-stub RELEASE_VERSION=vX.Y.Z"
	@echo "              make release RELEASE_VERSION=vX.Y.Z   (clean tree + HEAD == origin/$(MAIN_BRANCH))"

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
	go test -run=^$$ -fuzz=FuzzJSONRPCParse         -fuzztime=15s ./jsonrpc; \
	go test -run=^$$ -fuzz=FuzzMCPStreamableHTTP   -fuzztime=15s ./mcp; \
	go test -run=^$$ -fuzz=FuzzWebSocketFrameParse -fuzztime=15s ./websocket; \
	go test -run=^$$ -fuzz=FuzzCORSOriginMatch     -fuzztime=15s .; \
	go test -run=^$$ -fuzz=FuzzValidateEmail       -fuzztime=15s .

benchmark-load: ## Run reproducible loopback load profiles (BENCH_* variables tune the workload).
	./benchmarks/run_benchmarks.sh

clean:
	rm -rf bin hyperserve

version: ## Print the version string the next build would embed
	@echo $(VERSION)

changelog-lint: ## Validate topmost CHANGELOG.md entry for RELEASE_VERSION
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "changelog-lint: RELEASE_VERSION is required, e.g. make changelog-lint RELEASE_VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	@RELEASE_VERSION=$(RELEASE_VERSION) ./scripts/check-changelog-entry.sh

changelog-stub: ## Prepend a CHANGELOG.md entry skeleton for RELEASE_VERSION
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "changelog-stub: RELEASE_VERSION is required, e.g. make changelog-stub RELEASE_VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	@RELEASE_VERSION=$(RELEASE_VERSION) ./scripts/changelog-stub.sh

release-notes: ## Render GitHub Release notes from CHANGELOG.md for RELEASE_VERSION
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "release-notes: RELEASE_VERSION is required, e.g. make release-notes RELEASE_VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	@$(MAKE) --no-print-directory changelog-lint RELEASE_VERSION=$(RELEASE_VERSION) >&2
	@RELEASE_VERSION=$(RELEASE_VERSION) ./scripts/release-notes.sh

release-ci: ## Wait for and verify the push CI run for RELEASE_SHA (defaults to HEAD)
	@sha="$${RELEASE_SHA:-$$(git rev-parse HEAD)}"; \
		RELEASE_SHA="$$sha" ./scripts/wait-exact-sha-ci.sh

release-gate-test: ## Exercise the exact-SHA CI gate with deterministic fixtures
	@./scripts/wait-exact-sha-ci_test.sh

release-publish: ## Create GitHub Release page from CHANGELOG.md — RELEASE_VERSION required
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "release-publish: RELEASE_VERSION is required, e.g. make release-publish RELEASE_VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	@command -v gh >/dev/null 2>&1 || { echo "release-publish: gh CLI not on PATH; install gh" >&2; exit 1; }
	@$(MAKE) --no-print-directory changelog-lint RELEASE_VERSION=$(RELEASE_VERSION)
	@if ! git ls-remote --tags --exit-code origin $(RELEASE_VERSION) >/dev/null 2>&1; then \
		echo "release-publish: tag $(RELEASE_VERSION) is not on origin; run make release or push the tag first" >&2; \
		exit 1; \
	fi
	@remote_sha=$$(git ls-remote --tags origin 'refs/tags/$(RELEASE_VERSION)^{}' | awk 'NR == 1 { print $$1 }'); \
	if [ -z "$$remote_sha" ]; then \
		echo "release-publish: origin/$(RELEASE_VERSION) is not an annotated tag" >&2; \
		exit 1; \
	fi; \
	local_sha=$$(git rev-list -n 1 $(RELEASE_VERSION) 2>/dev/null || true); \
	if [ -z "$$local_sha" ] || [ "$$local_sha" != "$$remote_sha" ]; then \
		echo "release-publish: local and remote $(RELEASE_VERSION) do not resolve to the same commit" >&2; \
		echo "                 fetch and verify the immutable tag before recovery" >&2; \
		exit 1; \
	fi; \
	$(MAKE) --no-print-directory release-ci RELEASE_SHA="$$remote_sha"
	@notes=$$(mktemp -t hyperserve-release-notes.XXXXXX) && \
		trap 'rm -f $$notes' EXIT && \
		RELEASE_VERSION=$(RELEASE_VERSION) ./scripts/release-notes.sh > "$$notes" && \
		title="$${MESSAGE:-HyperServe $(RELEASE_VERSION)}" && \
		gh release create $(RELEASE_VERSION) --notes-file "$$notes" --title "$$title" --latest

release-smoke: ## Run the full local release gate before tagging
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "release-smoke: RELEASE_VERSION is required, e.g. make release-smoke RELEASE_VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	$(MAKE) -j$(RELEASE_TEST_JOBS) check
	go test ./...
	(cd examples/auth && go test ./...)
	$(MAKE) build VERSION=$(RELEASE_VERSION)
	@tmp=$$(mktemp -d); \
		trap 'rm -rf "$$tmp"' EXIT; \
		go run ./cmd/hyperserve-init --module example.com/hyperserve-release-smoke --out "$$tmp/app" --local-replace "$$(pwd)" >/dev/null; \
		grep -Fq 'github.com/osauer/hyperserve/v2 $(RELEASE_VERSION)' "$$tmp/app/go.mod" || { echo "release-smoke: scaffold does not require $(RELEASE_VERSION)" >&2; exit 1; }; \
		test -s "$$tmp/app/go.sum" || { echo "release-smoke: scaffold did not create go.sum" >&2; exit 1; }; \
		grep -Fq 'go 1.27' "$$tmp/app/go.mod" || { echo "release-smoke: scaffold does not use Go 1.27" >&2; exit 1; }; \
		grep -Fq 'FROM golang:1.27 AS builder' "$$tmp/app/Dockerfile" || { echo "release-smoke: scaffold Dockerfile does not use Go 1.27" >&2; exit 1; }; \
		(cd "$$tmp/app" && GOWORK=off go test -mod=readonly ./...)

# Tag, push, and publish a new release. RELEASE_VERSION is separate from the
# build-time VERSION variable so missing or malformed release input fails
# before any tag can be created.
release: ## Tag, push, and publish a release: make release RELEASE_VERSION=vX.Y.Z [MESSAGE="..."]
	@if [ -z "$(RELEASE_VERSION)" ]; then \
		echo "release: RELEASE_VERSION is required, e.g. make release RELEASE_VERSION=v1.2.3" >&2; \
		exit 1; \
	fi
	@if ! echo "$(RELEASE_VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "release: RELEASE_VERSION must look like vX.Y.Z (got $(RELEASE_VERSION))" >&2; \
		exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "release: working tree is dirty; commit or stash first" >&2; \
		git status --short >&2; \
		exit 1; \
	fi
	git fetch origin $(MAIN_BRANCH) --tags
	@head=$$(git rev-parse HEAD); \
	main=$$(git rev-parse origin/$(MAIN_BRANCH) 2>/dev/null) || { \
		echo "release: origin/$(MAIN_BRANCH) ref missing locally" >&2; \
		exit 1; \
	}; \
	if [ "$$head" != "$$main" ]; then \
		echo "release: HEAD ($$head) does not match origin/$(MAIN_BRANCH) ($$main); push your commits first" >&2; \
		exit 1; \
	fi
	@if git rev-parse --verify --quiet $(RELEASE_VERSION) >/dev/null; then \
		echo "release: tag $(RELEASE_VERSION) already exists locally" >&2; \
		exit 1; \
	fi
	@if git ls-remote --tags --exit-code origin $(RELEASE_VERSION) >/dev/null 2>&1; then \
		echo "release: tag $(RELEASE_VERSION) already exists on origin" >&2; \
		exit 1; \
	fi
	$(MAKE) changelog-lint RELEASE_VERSION=$(RELEASE_VERSION)
	$(MAKE) release-smoke RELEASE_VERSION=$(RELEASE_VERSION)
	@sha=$$(git rev-parse HEAD); \
		$(MAKE) --no-print-directory release-ci RELEASE_SHA="$$sha"
	@msg="$${MESSAGE:-HyperServe $(RELEASE_VERSION)}"; \
		git tag -a $(RELEASE_VERSION) -m "$$msg"
	git push origin HEAD:$(MAIN_BRANCH)
	git push origin $(RELEASE_VERSION)
	@msg="$${MESSAGE:-HyperServe $(RELEASE_VERSION)}"; \
		$(MAKE) release-publish RELEASE_VERSION=$(RELEASE_VERSION) MESSAGE="$$msg"
	@echo
	@echo "Released $(RELEASE_VERSION):"
	@echo "  https://github.com/osauer/hyperserve/releases/tag/$(RELEASE_VERSION)"

# Binding pre-commit gate: gofmt drift + go vet + staticcheck + govulncheck +
# go-fix/modernize drift. Mirrors the pattern in ../ibkr.
#
# Why staticcheck and govulncheck use `command -v` (not lazy install): these
# are developer-machine tools, expected to be installed once. The Makefile
# tells you the exact command if missing. Modernize is different — it's
# pinned via the `tool` directive in tools/go.mod and invoked from that module, so it
# auto-downloads on first use and stays reproducible across machines/CI.
check: vet staticcheck govulncheck govulncheck-tools modernize-check check-docs check-examples check-canonical-examples check-compatibility-examples mcp-conformance release-gate-test ## gofmt + vet + staticcheck + govulncheck + modernize-check + docs/example gates
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
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not on PATH; install: go install honnef.co/go/tools/cmd/staticcheck@v0.8.1" >&2; exit 1; }
	staticcheck ./...

govulncheck:
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck not on PATH; install: go install golang.org/x/vuln/cmd/govulncheck@v1.7.0" >&2; exit 1; }
	govulncheck ./...

govulncheck-tools: ## Scan the separated developer-tool module
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck not on PATH; install: go install golang.org/x/vuln/cmd/govulncheck@v1.7.0" >&2; exit 1; }
	govulncheck -C tools -tags=tools -scan=module

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

check-docs: ## Verify entry-point links and reject known stale example claims
	go test ./internal/doccheck

check-canonical-examples: ## Build and test every example in the main module
	go test ./examples/...

check-compatibility-examples: ## Build deprecated transport compatibility examples
	go test ./examples/mcp-sse

mcp-conformance: ## Verify Streamable HTTP with the official MCP Go SDK
	go -C tools test ./mcpconformance

# Idiom-drift gate. `go fix -diff` is the toolchain-native fixer (tracks the
# Go version pinned in go.mod); tools/go.mod's modernize runs the broader gopls
# analyzer suite (range N, wg.Go, b.Loop, maps.Copy, SplitSeq, any vs
# interface{}, etc.). Version of modernize is pinned via the `tool` directive
# in tools/go.mod, so the gate is reproducible without an `@latest` install step.
#
# Stream discipline + chatter filter:
#   - `go fix -diff` writes the unified diff to stdout, download chatter to
#     stderr → capture stdout (no redirect needed; stderr stays visible).
#   - `go -C tools tool modernize` writes diagnostics AND `go: downloading …` lines to
#     stderr (the latter when go.mod's tool deps aren't cached — every fresh
#     CI run hits this). Same stream means we can't separate by redirection;
#     instead we capture stderr via stream-swap and grep the chatter out.
modernize-check: ## go fix -diff + modernize gate (Go idiom drift vs go.mod's go version)
	@out=$$(go fix -diff ./...); \
	if [ -n "$$out" ]; then \
		echo "go fix found pending changes:"; echo "$$out"; \
		echo "apply with: make modernize"; exit 1; \
	fi
	@out=$$(go -C tools tool modernize github.com/osauer/hyperserve/v2/... 2>&1 1>/dev/null | grep -v '^go: downloading'); \
	if [ -n "$$out" ]; then \
		echo "modernize found pending changes:"; echo "$$out"; \
		echo "apply with: make modernize"; exit 1; \
	fi

modernize: ## Apply go fix + modernize rewrites in place
	go fix ./...
	go -C tools tool modernize -fix github.com/osauer/hyperserve/v2/...

fmt: ## gofmt -w over tracked / non-gitignored .go files (same scope as `make check`)
	@git ls-files --cached --others --exclude-standard '*.go' | \
		while IFS= read -r f; do [ -e "$$f" ] && printf '%s\n' "$$f"; done | \
		xargs gofmt -w
