
.DEFAULT_GOAL := du

.PHONY: build
build: ./bin/gitgum

SRCS := $(shell find . -name '*.go')

./bin/gitgum: $(SRCS) Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/gitgum ./src/cmd/gitgum/main.go
	# If we have upx installed, compress the binary
	if command -v upx >/dev/null 2>&1; then \
		upx ./bin/gitgum; \
	fi

.PHONY: du
du: ./bin/gitgum
	du -h ./bin/gitgum

.PHONY: clean
clean:
	rm -rf ./bin/gitgum
