# Get version from git tag or use dev
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%d_%H:%M:%S_UTC" || echo "unknown")

# Build flags
LDFLAGS := -ldflags "-X github.com/osauer/hyperserve.Version=$(VERSION) -X github.com/osauer/hyperserve.BuildHash=$(BUILD_HASH) -X github.com/osauer/hyperserve.BuildTime=$(BUILD_TIME)"

.PHONY: build
build:
	go build $(LDFLAGS) -o hyperserve .

.PHONY: install
install:
	go install $(LDFLAGS) .

.PHONY: test
test:
	go test -v ./...

.PHONY: clean
clean:
	rm -f hyperserve

.PHONY: version
version:
	@echo $(VERSION)