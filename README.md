# gitgum

A bunch of git commands with an interactive fuzzy-finder UI (used to be [gum](https://github.com/charmbracelet/gum), hence the name). The features are very tailored to the kind of workflows I have, but nothing work-specific is encoded in.

The picker is the in-tree [`src/fuzzyfinder`](src/fuzzyfinder) library — a slimmed-down fork of [`ktr0731/go-fuzzyfinder`](https://github.com/ktr0731/go-fuzzyfinder). No external `fzf` binary is required.

## Build

```bash
make build       # ./bin/gitgum and ./bin/fuzzyfinder
```

## Install

```bash
make install     # symlinks gitgum, gg, fuzzyfinder, ff into ~/.local/bin
```

Or by hand:

```bash
ln -s "$PWD/bin/gitgum"      ~/.local/bin/gitgum
ln -s "$PWD/bin/gitgum"      ~/.local/bin/gg
ln -s "$PWD/bin/fuzzyfinder" ~/.local/bin/fuzzyfinder
ln -s "$PWD/bin/fuzzyfinder" ~/.local/bin/ff
```

## Shell completions

```bash
# Fish
gitgum completion fish | source

# Bash
eval "$(gitgum completion bash)"

# Zsh
eval "$(gitgum completion zsh)"
```

## Commands

### `gitgum switch`

Pick a branch to switch to. Local and remote branches stream into the picker live, deduplicated. For remote selections, gitgum offers to retarget tracking, fast-forward / reset to the remote tip, or create a new tracking branch as appropriate.

### `gitgum status`

Print branches and a porcelain-formatted working-tree status with a header.

### `gitgum push`

Push the current branch. Picks a remote interactively when the branch has no upstream, or confirms a push to the existing tracking branch.

### `gitgum delete`

Interactively delete a local branch and optionally its remote tracking branch. The command will:
- Let you select a branch from the picker
- Warn before deleting `main` or `master`
- Prompt to switch branches if you're trying to delete the current branch
- Detect remote tracking branches and ask whether to delete them
- Attempt a safe delete first (`git branch -d`), falling back to force delete (`git branch -D`) with confirmation

### `gitgum clean`

Discard working-tree changes and untracked files. Flags: `--changes`, `--untracked`, `--ignored`, `--all`, `--yes`.

### `gitgum empty`

Create an empty commit (`--allow-empty`) and optionally push it. Useful for kicking CI.

### `gitgum checkout-pr`

Pick an open PR from a remote (`refs/pull/N/head` or `/merge`) and check it out locally.

### `gitgum replay-list A B`

List commits on branch A that have diverged from trunk/base branch B, in chronological order. Equivalent to:
```bash
git rev-list $(git merge-base A B)..A --reverse
```
Useful for identifying the feature commits on a branch that need to be replayed or cherry-picked onto another base.

### `gitgum release patch|minor|major`

Bump `VERSION` (or fall back to the latest `vX.Y.Z` tag), commit, and create an annotated tag. Refuses on a dirty working tree; prompts (default no) when not on `main`. Push is left manual so the result can be inspected. Also exposed as `make release BUMP=patch|minor|major`.

### `gitgum completion fish|bash|zsh`

Print the shell completion script for the given shell.

## `fuzzyfinder` (`ff`) — the standalone CLI

`bin/fuzzyfinder` is a small `fzf`-like CLI built on the same library. Reads items from stdin (one per line), writes the selection to stdout. Stream-friendly — items appear in the picker as they arrive:

```bash
find . -type f | ff
git branch --format='%(refname:short)' | ff -p 'branch> ' | xargs git checkout
```

Flags: `-m`/`--multi`, `-q`/`--query`, `-p`/`--prompt`, `--header`, `-1`/`--select-1`, `-v`/`--version`. Exit codes match `fzf` where reasonable: 0 success, 1 no match, 130 cancelled, 2 IO/flag error.

## Layout

- [`cmd/gitgum`](cmd/gitgum) — gitgum binary entry point
- [`cmd/fuzzyfinder`](cmd/fuzzyfinder) — `ff` binary entry point
- [`cmd/generate-version`](cmd/generate-version) — build-time tool that writes `src/version/version.go`
- [`src/commands`](src/commands) — one file per subcommand, each implements `flags.Commander`
- [`src/fuzzyfinder`](src/fuzzyfinder) — fork of `ktr0731/go-fuzzyfinder`; library kept independent of gitgum-specific code
- [`internal/git`](internal/git) — git operations (the `Repo` type for parallel-safe tests, plus CWD-based free functions)
- [`internal/cmdrun`](internal/cmdrun) — small `exec.Command` wrappers
- [`internal/ui`](internal/ui) — picker helpers (`Select`, `Confirm`, `ErrCancelled`)
- [`internal/strutil`](internal/strutil) — string helpers
- [`internal/testutil/temp_repo`](internal/testutil/temp_repo) — test fixtures

## Vendored licence

`src/fuzzyfinder/` derives from a vendored copy of `github.com/ktr0731/go-fuzzyfinder` under the MIT License. The original licence is preserved at [LICENSE-go-fuzzyfinder](LICENSE-go-fuzzyfinder).

## ToDo's

- [x] finish porting from bash version (what other commands do we want?)
- [x] integration tests with mock git repositories
- [ ] ? unify switch
- [ ] make switch work well when offline
- [ ] de-novo shell completions
