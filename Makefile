.SUFFIXES:

SRCS := $(shell find ./cmd ./src -name '*.go' ! -name 'version.go')

help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ./bin/gitgum  ## Build the gitgum binary (compressed with upx if available)

./bin/gitgum: $(SRCS) generate-version Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/gitgum ./cmd/gitgum
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/gitgum || echo "upx failed, skipping compression"; \
	fi

.PHONY: generate-version
generate-version:
	go generate ./src/version

.PHONY: du
du: ./bin/gitgum  ## Show the binary size
	du -h ./bin/gitgum

.PHONY: install
install: ./bin/gitgum  ## Symlink the binary into ~/.local/bin/gitgum
	mkdir -p $(HOME)/.local/bin
	ln -sf "$(PWD)/bin/gitgum" "$(HOME)/.local/bin/gitgum"
	ln -sf "$(PWD)/bin/gitgum" "$(HOME)/.local/bin/gg"

.PHONY: test
test:  ## Run the test suite (and the vendored go-fuzzyfinder tests)
	@if command -v gotest >/dev/null 2>&1; then \
		gotest ./...; \
	else \
		go test ./...; \
	fi
	$(MAKE) -C my-vendor/go-fuzzyfinder unit-test

.PHONY: check
check:  ## go vet across the module
	go vet ./...

.PHONY: fmt
fmt:  ## gofmt the tree in place
	gofmt -s -w ./cmd ./src

.PHONY: fmt-check
fmt-check:  ## Verify gofmt without modifying files
	@out=$$(gofmt -s -l ./cmd ./src); \
	if [ -n "$$out" ]; then \
		echo "Unformatted files:"; echo "$$out"; exit 1; \
	fi

.PHONY: verify
verify: fmt-check check test  ## Pre-commit gate: fmt-check, vet, test
	@echo "All checks passed."

.PHONY: clean
clean:  ## Remove build artifacts and generated files
	rm -f ./bin/gitgum
	rm -f ./src/version/version.go
