.SUFFIXES:

SRCS := $(shell find ./cmd ./internal ./src -name '*.go' ! -name 'version.go')

help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ./bin/gitgum ./bin/fuzzyfinder  ## Build all binaries (compressed with upx if available)

./bin/gitgum: $(SRCS) generate-version Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/gitgum ./cmd/gitgum
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/gitgum || echo "upx failed, skipping compression"; \
	fi

./bin/fuzzyfinder: $(SRCS) Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/fuzzyfinder ./cmd/fuzzyfinder
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/fuzzyfinder || echo "upx failed, skipping compression"; \
	fi

.PHONY: generate-version
generate-version:
	go generate ./src/version

.PHONY: du
du: ./bin/gitgum ./bin/fuzzyfinder  ## Show binary sizes
	du -h ./bin/gitgum ./bin/fuzzyfinder

.PHONY: install
install: ./bin/gitgum ./bin/fuzzyfinder  ## Symlink binaries into ~/.local/bin
	mkdir -p $(HOME)/.local/bin
	ln -sf "$(PWD)/bin/gitgum" "$(HOME)/.local/bin/gitgum"
	ln -sf "$(PWD)/bin/gitgum" "$(HOME)/.local/bin/gg"
	ln -sf "$(PWD)/bin/fuzzyfinder" "$(HOME)/.local/bin/fuzzyfinder"
	ln -sf "$(PWD)/bin/fuzzyfinder" "$(HOME)/.local/bin/ff"

.PHONY: test
test:  ## Run the test suite with race detector
	@if command -v gotest >/dev/null 2>&1; then \
		gotest -race ./...; \
	else \
		go test -race ./...; \
	fi

.PHONY: check
check:  ## go vet across the module
	go vet ./...

.PHONY: fmt
fmt:  ## gofmt the tree in place
	gofmt -s -w ./cmd ./internal ./src

.PHONY: fmt-check
fmt-check:  ## Verify gofmt without modifying files
	@out=$$(gofmt -s -l ./cmd ./internal ./src); \
	if [ -n "$$out" ]; then \
		echo "Unformatted files:"; echo "$$out"; exit 1; \
	fi

.PHONY: coverage
coverage:  ## Run tests with coverage profile
	go test -coverpkg ./... -covermode=atomic -coverprofile=coverage.txt -race ./...

.PHONY: coverage-web
coverage-web: coverage  ## Open coverage report in browser
	go tool cover -html=coverage.txt

.PHONY: verify
verify: fmt-check check test  ## Pre-commit gate: fmt-check, vet, test
	@echo "All checks passed."

.PHONY: clean
clean:  ## Remove build artifacts and generated files
	rm -f ./bin/gitgum ./bin/fuzzyfinder
	rm -f ./src/version/version.go
	rm -f ./coverage.txt
