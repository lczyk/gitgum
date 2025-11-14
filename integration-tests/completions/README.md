# Shell Completion Integration Tests

This directory contains integration tests for `gitgum` shell completions. Unlike the unit tests in `src/commands/*_test.go`, these tests run in actual shell environments inside containers to verify that completions work correctly in real-world scenarios.

## Overview

The test suite:
- Builds the `gitgum` binary
- Spins up Ubuntu 24.04 containers for Bash, Fish, and Zsh
- Generates completion scripts inside each container
- Sources the completions and invokes completion hooks
- Validates that completions work without errors

## Prerequisites

### Required Tools
- **Container runtime**: Docker or Podman
- **Make** (optional, for building via Makefile)
- **Go 1.21+** (to build the binary)

### Supported Container Backends
- Docker (default)
- Podman

## Quick Start

Run all completion tests with Docker:

```bash
./run-completion-tests.sh
```

Run with Podman instead:

```bash
COMPLETIONS_BACKEND=podman ./run-completion-tests.sh
```

## Directory Structure

```
integration-tests/completions/
├── run-completion-tests.sh    # Main orchestration script
├── images/                     # Dockerfiles for each shell environment
│   ├── Dockerfile.bash         # Bash environment (Ubuntu 24.04 + git + fzf)
│   ├── Dockerfile.fish         # Fish environment (+ fish shell)
│   └── Dockerfile.zsh          # Zsh environment (+ zsh shell)
├── tests/                      # Shell-specific test scripts
│   ├── test-bash.sh            # Bash completion tests
│   ├── test-fish.sh            # Fish completion tests
│   └── test-zsh.sh             # Zsh completion tests
├── bin/                        # Temporary binary storage (gitignored)
└── README.md                   # This file
```

## How It Works

1. **Build Phase**: The orchestration script builds `bin/gitgum` using `make` or `go build`
2. **Image Build**: Builds Docker/Podman images for each shell environment
3. **Test Execution**: For each shell:
   - Mounts the completion test directory into the container
   - Runs `gitgum completion <shell>` to generate completions
   - Sources the completion script in the target shell
   - Invokes completion hooks to verify they execute without errors
   - Checks for expected completion markers (functions, command registrations)
4. **Validation**: Tests verify:
   - Placeholder `__GITGUM_CMD__` is properly replaced
   - Completion functions are defined
   - Completion hooks execute without errors
   - Key completion entries are present

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `COMPLETIONS_BACKEND` | Container runtime to use (`docker` or `podman`) | `docker` |
| `COMPLETIONS_IMAGE_PREFIX` | Prefix for container image names | `gitgum-completions` |

### Examples

```bash
# Use Podman
COMPLETIONS_BACKEND=podman ./run-completion-tests.sh

# Custom image prefix
COMPLETIONS_IMAGE_PREFIX=my-gitgum ./run-completion-tests.sh
```

## Rebuilding Container Images

If you modify the Dockerfiles or need to rebuild images manually:

```bash
# Build all images with Docker
docker build -f images/Dockerfile.bash -t gitgum-completions-bash images/
docker build -f images/Dockerfile.fish -t gitgum-completions-fish images/
docker build -f images/Dockerfile.zsh -t gitgum-completions-zsh images/

# Or with Podman
podman build -f images/Dockerfile.bash -t gitgum-completions-bash images/
podman build -f images/Dockerfile.fish -t gitgum-completions-fish images/
podman build -f images/Dockerfile.zsh -t gitgum-completions-zsh images/
```

The orchestration script rebuilds images automatically, but manual rebuilds can be useful for debugging.

## Test Details

### Bash Tests (`test-bash.sh`)
- Generates completion via `gitgum completion bash`
- Verifies `_gitgum_completion` function exists
- Sources the completion script
- Invokes the completion hook with simulated `COMP_*` variables
- Checks that the completion function executes without errors

### Fish Tests (`test-fish.sh`)
- Generates completion via `gitgum completion fish`
- Verifies `__fish_gitgum_needs_command` function exists
- Sources the completion script
- Uses Fish's `complete -C` to test completions
- Validates that `switch` command appears in completions

### Zsh Tests (`test-zsh.sh`)
- Generates completion via `gitgum completion zsh`
- Verifies `#compdef` directive and `_gitgum` function
- Initializes Zsh completion system (`compinit`)
- Sources the completion script
- Attempts to invoke the completion function

## Extending the Tests

### Adding a New Shell

1. Create a new Dockerfile in `images/Dockerfile.<shell>`
2. Create a test script in `tests/test-<shell>.sh`
3. Add the shell name to the `SHELLS` array in `run-completion-tests.sh`

### Adding More Assertions

Edit the shell-specific test scripts in `tests/` to add:
- More completion hook invocations
- Validation of specific completion suggestions
- Edge case testing (e.g., multi-word completions)

### Adding a New Container Backend

Modify `run-completion-tests.sh` to support additional backends:
1. Update the backend validation in `check_backend()`
2. Ensure command compatibility (most backends use similar syntax)

## Troubleshooting

### Container Backend Not Found
```
ERROR: Container backend 'docker' not found
```
**Solution**: Install Docker/Podman or set `COMPLETIONS_BACKEND` to an available runtime.

### Image Build Failures
Check that:
- You have network access (to download Ubuntu base image and packages)
- The container backend has sufficient permissions
- Dockerfiles are valid and haven't been corrupted

### Test Failures
- **Check logs**: The test scripts print detailed output
- **Run manually**: Execute the test script directly in a shell to debug
- **Inspect images**: Run an interactive session to explore:
  ```bash
  docker run -it --rm gitgum-completions-fish /usr/bin/fish
  ```

### Binary Not Found
If tests fail with "gitgum binary not found":
- Ensure `make build` or `go build` succeeds
- Check that `bin/gitgum` is executable
- Verify the binary is copied to `integration-tests/completions/bin/`

## CI Integration

To run these tests in CI:

```yaml
# Example GitHub Actions job
- name: Run completion integration tests
  run: |
    cd integration-tests/completions
    ./run-completion-tests.sh
```

For Podman-based CI (e.g., some container registries):

```yaml
- name: Run completion integration tests
  run: |
    cd integration-tests/completions
    COMPLETIONS_BACKEND=podman ./run-completion-tests.sh
```

## Differences from Unit Tests

| Aspect | Unit Tests (`*_test.go`) | Integration Tests |
|--------|-------------------------|-------------------|
| **Scope** | Individual functions and logic | End-to-end shell behavior |
| **Environment** | Go test runtime | Real shell in container |
| **Speed** | Fast (~seconds) | Slower (image build + container startup) |
| **Dependencies** | Mock/stub external calls | Real `fzf`, `git`, shell completion systems |
| **Purpose** | Verify code correctness | Verify actual user experience |

Both test suites are important and complementary.

## License

Same as the main `gitgum` project.
