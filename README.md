# gitgum - Go Implementation

This directory contains the Go rewrite of gitgum. The implementation currently supports the `switch` and `completion` commands.

## Prerequisites

- Go 1.21 or later
- `git` (system command)
- `fzf` (for interactive selections)

## Build

From the repository root:

```bash
make
```

This will create the binary at `bin/gitgum`.

Alternatively, build manually:

```bash
cd src/cmd/gitgum
go build -o ../../../bin/gitgum
```

## Install

Link the binary to your PATH:

```bash
ln -s "$PWD/bin/gitgum" ~/.local/bin/gitgum
# Or use an alias like 'gg':
ln -s "$PWD/bin/gitgum" ~/.local/bin/gg
```

## Usage

### Switch Command

Interactively switch branches with three modes:

```bash
gitgum switch
```

Options:
1. **Switch to an existing local branch** - Select from local branches
2. **Switch to a remote branch** - Create local tracking branch from remote
3. **Create a new branch** - Not yet implemented

### Completion Command

Generate shell completions:

```bash
# Fish
gitgum completion fish | source

# Bash
eval "$(gitgum completion bash)"

# Zsh
eval "$(gitgum completion zsh)"
```

## Testing

Run the test suite:

```bash
go test ./...
```

Run tests for a specific package:

```bash
go test ./src/commands -v
```

## Architecture

The Go implementation mirrors the bash version but uses:
- `fzf` for interactive selections (instead of `gum`)
- System `git` commands for all git operations
- No dry-run flag support (excluded by design)

### File Structure

- `src/cmd/gitgum/main.go` - CLI dispatcher and command registration
- `src/commands/switch.go` - Branch switching logic
- `src/commands/completion.go` - Shell completion generation
- `src/completions/assets/` - Embedded completion templates (bash, fish, zsh)
- `cmd/generate-completions/` - Generator for embedding completion assets
- `src/commands/*_test.go` - Test suites

## Differences from Bash Version

1. Uses `fzf` instead of `gum` for interactive prompts
2. No dry-run flag (`-n`, `--dry-run`) support
3. Only `switch` and `completion` commands implemented initially
4. **Completion templates are now embedded in the binary** (no external files needed at runtime)

## Future Work

- Implement remaining commands (push, delete, commit, status, tree, merge-into)
- Add `switch new` mode for creating new branches
- Enhanced error handling and validation
- Integration tests with mock git repositories
