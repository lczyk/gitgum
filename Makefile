
.DEFAULT_GOAL := du

.PHONY: build
build: ./bin/gitgum

SRCS := $(shell find . -name '*.go')

./bin/gitgum: $(SRCS) ./cmd/gitgum/main.go Makefile go.mod go.sum ./src/completions/generated.go ./src/version/version.go
	mkdir -p ./bin
	go build -o ./bin/gitgum ./cmd/gitgum
	# If we have upx installed, compress the binary
	if command -v upx >/dev/null 2>&1; then \
		upx ./bin/gitgum; \
	fi

./src/completions/generated.go: ./src/completions/generate.go $(SRCS) Makefile go.mod go.sum
	go generate ./src/completions

./src/version/version.go: ./src/version/generate.go VERSION Makefile
	go generate ./src/version

.PHONY: du
du: ./bin/gitgum
	du -h ./bin/gitgum

.PHONY: test
test:
	@if command -v gotest >/dev/null 2>&1; then \
		gotest ./...; \
	else \
		go test ./...; \
	fi

	# Run make test in go-fuzzyfinder to ensure it works with the latest changes
	$(MAKE) -C my-vendor/go-fuzzyfinder unit-test

.PHONY: clean
clean:
	rm -rf ./bin/gitgum
	rm -rf ./src/completions/generated.go
	rm -rf ./src/version/version.go
