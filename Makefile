# Get version from git tag or use dev
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build flags
LDFLAGS := -ldflags "-X github.com/osauer/hyperserve.Version=$(VERSION)"

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