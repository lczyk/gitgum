# Makefile for gitgum completion integration tests
# Tests completions in containerized bash, fish, and zsh environments

.DEFAULT_GOAL := test

# Configuration
PROJECT_ROOT := ../..
BIN_DIR := ./bin
IMAGES_DIR := ./images
TESTS_DIR := ./tests

# Container backend (docker or podman)
CONTAINER_BACKEND ?= docker
IMAGE_PREFIX ?= gitgum-completions

# Shells to test
SHELLS := bash fish zsh

# Color output
RED := \033[0;31m
YELLOW := \033[1;33m
GREEN := \033[0;32m
NC := \033[0m

.PHONY: build-binary
build-binary:
	$(MAKE) -C $(PROJECT_ROOT) build
	mkdir -p $(BIN_DIR)
	cp $(PROJECT_ROOT)/bin/gitgum $(BIN_DIR)/gitgum
	chmod +x $(BIN_DIR)/gitgum

.PHONY: build-images
build-images: build-bash build-fish build-zsh

.PHONY: build-bash
build-bash: $(IMAGES_DIR)/Dockerfile.bash
	$(CONTAINER_BACKEND) build \
		-t $(IMAGE_PREFIX)-bash \
		-f $(IMAGES_DIR)/Dockerfile.bash \
		$(IMAGES_DIR)

.PHONY: build-fish
build-fish: $(IMAGES_DIR)/Dockerfile.fish
	$(CONTAINER_BACKEND) build \
		-t $(IMAGE_PREFIX)-fish \
		-f $(IMAGES_DIR)/Dockerfile.fish \
		$(IMAGES_DIR)

.PHONY: build-zsh
build-zsh: $(IMAGES_DIR)/Dockerfile.zsh
	$(CONTAINER_BACKEND) build \
		-t $(IMAGE_PREFIX)-zsh \
		-f $(IMAGES_DIR)/Dockerfile.zsh \
		$(IMAGES_DIR)

.PHONY: test-bash
test-bash: build-binary build-bash
	$(CONTAINER_BACKEND) run --rm \
		-v $(CURDIR):/work \
		-w /work \
		$(IMAGE_PREFIX)-bash \
		/work/tests/test-bash.sh

.PHONY: test-fish
test-fish: build-binary build-fish
	$(CONTAINER_BACKEND) run --rm \
		-v $(CURDIR):/work \
		-w /work \
		$(IMAGE_PREFIX)-fish \
		/work/tests/test-fish.fish

.PHONY: test-zsh
test-zsh: build-binary build-zsh
	$(CONTAINER_BACKEND) run --rm \
		-v $(CURDIR):/work \
		-w /work \
		$(IMAGE_PREFIX)-zsh \
		/work/tests/test-zsh.sh

# test all by calling make with test-<shell> for each shell
.PHONY: test
test:
	$(MAKE) test-bash
	$(MAKE) test-fish
	$(MAKE) test-zsh
	printf "$(GREEN)INFO$(NC) ✓ All completion tests PASSED\n"

.PHONY: interactive-bash
interactive-bash: build-binary build-bash
	$(CONTAINER_BACKEND) run --rm -it \
		-v $(CURDIR):/work \
		-w /work \
		$(IMAGE_PREFIX)-bash \
		bash -c ' \
			ln -s /work/bin/gitgum /usr/local/bin/gitgum; \
			/work/bin/gitgum completion bash > /etc/bash_completion.d/gitgum.bash; \
			bash --login \
		'

.PHONY: interactive-fish
interactive-fish: build-binary build-fish
	$(CONTAINER_BACKEND) run --rm -it \
		-v $(CURDIR):/work \
		-w /work \
		$(IMAGE_PREFIX)-fish \
		fish -c ' \
			ln -s /work/bin/gitgum /usr/local/bin/gitgum; \
			/work/bin/gitgum completion fish > /etc/fish/completions/gitgum.fish; \
			fish --login \
		'

# NOTE: in zsh we source the completion script in .zshrc
.PHONY: interactive-zsh
interactive-zsh: build-binary build-zsh
	$(CONTAINER_BACKEND) run --rm -it \
		-v $(CURDIR):/work \
		-w /work \
		$(IMAGE_PREFIX)-zsh \
		zsh -c ' \
			ln -s /work/bin/gitgum /usr/local/bin/gitgum; \
			echo "autoload -U compinit; compinit" >> /etc/zshrc; \
			echo ". `/work/bin/gitgum completion zsh`" >> /etc/zshrc; \
			zsh --login \
		'

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)/gitgum
	$(CONTAINER_BACKEND) rmi $(IMAGE_PREFIX)-bash 2>/dev/null || true
	$(CONTAINER_BACKEND) rmi $(IMAGE_PREFIX)-fish 2>/dev/null || true
	$(CONTAINER_BACKEND) rmi $(IMAGE_PREFIX)-zsh 2>/dev/null || true

.PHONY: clean-images
clean-images: check-backend
	for shell in $(SHELLS); do \
		$(CONTAINER_BACKEND) rmi $(IMAGE_PREFIX)-$$shell 2>/dev/null || true; \
	done
