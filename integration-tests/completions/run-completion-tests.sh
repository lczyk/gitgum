#!/bin/bash
#spellchecker: ignore gitgum

# Integration test runner for gitgum shell completions
# Tests completions in containerized bash, fish, and zsh environments

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPLETIONS_DIR="$SCRIPT_DIR"
IMAGES_DIR="$COMPLETIONS_DIR/images"
TESTS_DIR="$COMPLETIONS_DIR/tests"
BIN_DIR="$COMPLETIONS_DIR/bin"

# Container backend (docker or podman)
CONTAINER_BACKEND="${COMPLETIONS_BACKEND:-docker}"

# Image name prefix
IMAGE_PREFIX="${COMPLETIONS_IMAGE_PREFIX:-gitgum-completions}"

# Shells to test
SHELLS=("bash" "fish" "zsh")

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

error() { echo -e "${RED}ERROR${NC} $*"; }
warn() { echo -e "${YELLOW}WARNING${NC} $*"; }
info() { echo -e "${GREEN}INFO${NC} $*"; }

# Track test results
declare -A TEST_RESULTS


# Helper function: check if container backend is available
check_backend() {
    if ! command -v "$CONTAINER_BACKEND" &>/dev/null; then
        error "Container backend '$CONTAINER_BACKEND' not found"
        warn "Install $CONTAINER_BACKEND or set COMPLETIONS_BACKEND to 'docker' or 'podman'"
        exit 1
    fi
    info "Using container backend: $CONTAINER_BACKEND"
}

# Helper function: build gitgum binary
build_gitgum() {
    warn "Building gitgum binary..."
    make -C "$PROJECT_ROOT" build
    
    # Copy to integration test bin directory
    mkdir -p "$BIN_DIR"
    cp "$PROJECT_ROOT/bin/gitgum" "$BIN_DIR/gitgum"
    chmod +x "$BIN_DIR/gitgum"
    
    info "✓ Binary built and copied to $BIN_DIR/gitgum"
}

# Helper function: build container image for a shell
build_image() {
    local shell=$1
    local image_name="${IMAGE_PREFIX}-${shell}"
    local dockerfile="$IMAGES_DIR/Dockerfile.${shell}"
    
    if [[ ! -f "$dockerfile" ]]; then
        error "Dockerfile not found: $dockerfile"
        return 1
    fi
    
    warn "Building image for $shell..."
    
    if ! "$CONTAINER_BACKEND" build \
        -t "$image_name" \
        -f "$dockerfile" \
        "$IMAGES_DIR"; then
        error "Failed to build image for $shell"
        return 1
    fi
    
    info "✓ Built image: $image_name"
    return 0
}

# Helper function: run test in container
run_test() {
    local shell=$1
    local image_name="${IMAGE_PREFIX}-${shell}"
    local test_script="test-${shell}.sh"
    
    warn "\n=== Testing $shell completions ==="
    
    # Make test script executable
    chmod +x "$TESTS_DIR/$test_script"
    
    # Run container with mounted work
    if "$CONTAINER_BACKEND" run --rm \
        -v "$COMPLETIONS_DIR:/work" \
        -w /work \
        "$image_name" \
        "/work/tests/$test_script"; then
        info "✓ $shell test PASSED"
        TEST_RESULTS[$shell]="PASS"
        return 0
    else
        error "l test FAILED"
        TEST_RESULTS[$shell]="FAIL"
        return 1
    fi
}

# Main execution
main() {
    info "=== Gitgum Completion Integration Tests ==="
    echo
    
    # Check prerequisites
    check_backend
    
    # Build binary
    build_gitgum
    echo
    
    # Build images and run tests
    local failed=0
    
    for shell in "${SHELLS[@]}"; do
        if build_image "$shell"; then
            if ! run_test "$shell"; then
                ((failed++))
            fi
        else
            error "g $shell test due to image build failure"
            TEST_RESULTS[$shell]="SKIP"
            ((failed++))
        fi
        echo
    done
    
    # Print summary
    warn "=== Test Summary ==="
    for shell in "${SHELLS[@]}"; do
        local result="${TEST_RESULTS[$shell]:-UNKNOWN}"
        case "$result" in
            PASS)
                info "  $shell: ✓ PASSED"
                ;;
            FAIL)
                error "l: ✗ FAILED"
                ;;
            SKIP)
                warn "  $shell: ⊘ SKIPPED"
                ;;
        esac
    done
    echo
    
    if [[ $failed -eq 0 ]]; then
        info "All tests passed!"
        exit 0
    else
        error " test(s) failed"
        exit 1
    fi
}

# Show help
if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]]; then
    cat <<EOF
Usage: $0 [OPTIONS]

Run integration tests for gitgum shell completions in containers.

OPTIONS:
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    COMPLETIONS_BACKEND         Container backend to use (docker|podman)
                                Default: docker
    
    COMPLETIONS_IMAGE_PREFIX    Prefix for container image names
                                Default: gitgum-completions

EXAMPLES:
    # Run with Docker (default)
    ./run-completion-tests.sh
    
    # Run with Podman
    COMPLETIONS_BACKEND=podman ./run-completion-tests.sh
    
    # Use custom image prefix
    COMPLETIONS_IMAGE_PREFIX=my-gitgum ./run-completion-tests.sh

REBUILDING IMAGES:
    If you need to rebuild the container images manually:
    
    docker build -f images/Dockerfile.bash -t gitgum-completions-bash images/
    docker build -f images/Dockerfile.fish -t gitgum-completions-fish images/
    docker build -f images/Dockerfile.zsh -t gitgum-completions-zsh images/
    
    (Replace 'docker' with 'podman' if using Podman)

EOF
    exit 0
fi

# Run main
main
