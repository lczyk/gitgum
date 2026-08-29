.SUFFIXES:

SRCS := $(shell find ./cmd ./internal ./src -name '*.go' ! -name 'version.go')

# Lint tools run via `go run` rather than being installed, matching how
# generate-version is invoked below: CI gets them from the module cache with
# no extra step. Pinned, so a new release cannot fail the build unannounced.
#
# staticcheck reads a toolchain's export data, so its pin has a floor as well
# as a ceiling: too old for the Go running it and every package fails to
# import. v0.8 covers 1.27 and needs 1.26 to build, which is the workflow's
# go-version.
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1
DEADCODE    := golang.org/x/tools/cmd/deadcode@v0.48.0

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

# generate-version is a prerequisite of both binaries, not just gitgum: both
# cmd/ mains import src/version, so building either one against a tree where
# version.go has not been generated (fresh clone, post-`make clean`) fails to
# resolve the package. It being phony is deliberate -- version.go embeds the
# current commit sha, so it is regenerated every build rather than dated
# against sources, which is also why SRCS excludes it.
./bin/fuzzyfinder: $(SRCS) generate-version Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/fuzzyfinder ./cmd/fuzzyfinder
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/fuzzyfinder || echo "upx failed, skipping compression"; \
	fi

.PHONY: generate-version
generate-version:
	go run github.com/lczyk/version/go/cmd/generate-version -out ./src/version/version.go -pkg version

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
test: generate-version  ## Run the test suite with race detector
	@if command -v gotest >/dev/null 2>&1; then \
		gotest -race ./...; \
	else \
		go test -race ./...; \
	fi

.PHONY: lint
lint:  ## go vet + staticcheck + deadcode + gofmt check (no writes)
	go vet ./...
	go run $(STATICCHECK) ./...
	@# deadcode exits 0 whatever it finds, so its output is the signal.
	@# -test counts test functions as entry points; without it every helper
	@# only tests use reads as unreachable.
	@out=$$(go run $(DEADCODE) -test ./...); \
	if [ -n "$$out" ]; then \
		echo "Unreachable code:"; echo "$$out"; exit 1; \
	fi
	@out=$$(gofmt -s -l ./cmd ./internal ./src); \
	if [ -n "$$out" ]; then \
		echo "Unformatted files:"; echo "$$out"; exit 1; \
	fi

.PHONY: format
format:  ## gofmt the tree in place
	gofmt -s -w ./cmd ./internal ./src

.PHONY: bench
bench:  ## Run benchmarks (override scope/duration: PKG=… BENCH=… BENCHTIME=…)
	go test -run '^$$' -bench '$(or $(BENCH),.)' -benchmem -benchtime '$(or $(BENCHTIME),1s)' $(or $(PKG),./...)

.PHONY: cover
cover:  ## Coverage profile + HTML file (cover.out, cover.html)
	go test -coverpkg=./... -coverprofile=cover.out -race ./...
	go tool cover -func=cover.out
	go tool cover -html=cover.out -o cover.html

.PHONY: cover-open
cover-open: cover  ## Run coverage and open the HTML report in a browser
	go tool cover -html=cover.out

.PHONY: verify
verify: lint test  ## Pre-commit gate: lint, test
	@echo "All checks passed."

.PHONY: clean
clean:  ## Remove build artifacts and generated files
	rm -f ./bin/gitgum ./bin/fuzzyfinder
	rm -f ./src/version/version.go
	rm -f ./cover.out ./cover.html
