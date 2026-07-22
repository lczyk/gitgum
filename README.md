# gitgum

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/lczyk/gitgum)
![GitHub Tag](https://img.shields.io/github/v/tag/lczyk/gitgum?label=release)
[![lint_and_test](https://github.com/lczyk/gitgum/actions/workflows/lint_and_test.yml/badge.svg)](https://github.com/lczyk/gitgum/actions/workflows/lint_and_test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/lczyk/goruby)](https://goreportcard.com/report/github.com/lczyk/goruby)

A bunch of git commands with an interactive fuzzy-finder UI (used to be [gum](https://github.com/charmbracelet/gum), hence the name). The features are very tailored to the kind of workflows I have, but nothing work-specific is encoded in.

## Requirements

- `git` >= 2.35.2 (Mar 2022). gg checks the installed version on first use and fails loudly below the floor. Lower bound is driven by the `safe.directory` config gating gg uses to keep parsing stable across machines.

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

### `gitgum clone URL [DIR]`

Clone a repository like `git clone`, but pre-applying the `gg doctor` opinions so a fresh clone is already clean: the remote is named after the forge user/org (e.g. `canonical`) rather than `origin`, and the clone lands in a directory named exactly after the repo.

It accepts the ways a repo can be spelled and normalises them:

- full urls -- `https://...`, `ssh://...`, `git://...`, and scp `git@host:user/repo` (cloned verbatim, so an ssh url is not downgraded to https)
- host shorthand -- `github.com/user/repo`, `www.github.com/user/repo` (scheme and `www.` filled in / stripped)
- bare shorthand -- `user/repo`, resolved by probing github, gitlab and codeberg for the repo's existence. Zero matches errors; one is used directly; several prompt a picker.

`--depth N` makes a shallow clone. A url on a host gitgum doesn't model (self-hosted forge, bitbucket, a local path) falls back to plain `git clone` with git's default `origin`.

### `gitgum switch`

Pick a branch to switch to. Local and remote branches stream into the picker live, deduplicated. For remote selections, gitgum offers to retarget tracking, fast-forward / reset to the remote tip, or create a new tracking branch as appropriate.

### `gitgum branch`

Create a new branch and switch to it. Pick the start point from the same live picker `switch` uses -- including the current branch, and branches checked out in other worktrees -- then type the new name. Existing local branches are listed while you type, so a colliding name shows up before you commit to it; picking a remote branch as the start point creates the new branch with `--no-track`, so a later `gg push` won't target someone else's branch.

On a detached HEAD the picker offers `HEAD (detached at <sha>)` as a start point, which is how commits made there stop being unreachable. `gg switch` shows the same row but refuses it -- a detached HEAD is not a branch to switch to.

### `gitgum status`

Print one or more status sections, named by a comma-separated argument:

- `branch` (`b`) -- local branches with their upstreams
- `remote` (`r`) -- configured remotes
- `worktree` (`w`) -- linked worktrees
- `changes` (`c`) -- a tree-formatted view of the working-tree changes (modified files get an inline `(+a,-d)` line-change count)
- `head` (`h`) -- the current branch and its upstream, as `* (origin/)main [ahead 7]`

Sections render in the order given, and a repeated section moves to its last position -- `b,r,b` renders as `r,b`. Surrounding whitespace is ignored, so `gg status "b, r"` works. A single section prints bare; two or more get headers. `gg status` with no argument is `gg status changes,head`.

On a detached HEAD, `head` names the commit and every ref that contains it -- local branches first, then remote-only branches, then tags, closest first. The `~N` suffix is the commit's distance below that ref, and is omitted when the commit sits off the ref's first-parent chain:

```
* HEAD cf10b5f (origin/)feat/x~1     # exactly one containing ref
* HEAD cf10b5f (no branch)           # a commit no ref can reach
* HEAD cf10b5f                       # several
    (origin/)feat/x~1
    feat/y~3
    v1.2.0~4
```

Pass `--flat` for a porcelain list instead of the change tree. Pass `--follow` / `-f` (optional `=N` interval, default 2s, min 1) to refresh the selected sections in an alt-screen with `j/k g/G PgUp/PgDn` scroll and `q` to exit; no remote ops run in follow mode.

### `gitgum tree`

Print a colored commit graph across all branches, with the tip at the bottom (right above the next prompt) so it stays visible after the output scrolls. Roughly:
```bash
git log --graph --oneline --all --decorate   # then reverse + flip diagonals
```
Defaults to the last two weeks. Override with `--since=<expr>` (any value `git log --since` accepts: `1m`, `yesterday`, `2024-01-01`, `"3 weeks ago"`). Pass `--since=` (empty), or `--all` / `-a`, for the full history. Pass `--follow` / `-f` (optional `=N` interval) for an auto-refreshing alt-screen view with `j/k g/G` scroll.

### `gitgum push`

Push the current branch. Picks a remote interactively when the branch has no upstream, or confirms a push to the existing tracking branch.

### `gitgum pull`

Fetch the current branch's upstream and integrate it. Reports "already up to date" when there's nothing new; otherwise picks the strategy interactively -- fast-forward only (the default, refuses a merge commit), rebase, or merge. Uncommitted tracked changes are stashed before the pull and popped after, matching the other commands. A shallow clone stays shallow: only new commits are fetched, old history is not backfilled. Errors if the branch has no upstream configured.

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

Bump `VERSION` (or fall back to the latest `vX.Y.Z` tag), commit, and create an annotated tag. Prompts (default no) when not on `main`. If the working tree has tracked uncommitted changes, prompts to auto-stash them (untracked files are left alone, partial-hunk staging is preserved on restore). Push is left manual so the result can be inspected.

Before committing, scans every tracked text file for lines mentioning the current version (line contains the word "version" + a boundaried token match), and offers them in a multi-select picker. Picked lines get a plain string-replace bump (no language-specific parsing) and ride in the release commit. Esc / no picks skips the auto-edits; the release proceeds either way. Binary files and files larger than 4 MiB are soft-skipped.

### `gitgum completion fish|bash|zsh`

Print the shell completion script for the given shell.

## `fuzzyfinder` (`ff`) — the standalone CLI

`bin/fuzzyfinder` is a small `fzf`-like CLI built on the same library. Reads items from stdin (one per line), writes the selection to stdout. Stream-friendly — items appear in the picker as they arrive:

```bash
find . -type f | ff
git branch --format='%(refname:short)' | ff -p 'branch> ' | xargs git checkout
```

Flags: `-m`/`--multi`, `-q`/`--query`, `-p`/`--prompt`, `--header`, `-1`/`--select-1`, `--reverse`, `--fast`, `--height=N`, `-v`/`--version`. Exit codes match `fzf` where reasonable: 0 success, 1 no match, 130 cancelled, 2 IO/flag error.

`--height=N` renders the picker inline at the bottom N rows of the terminal (preserves prior output above) instead of taking over the full screen. `0` (default) is fullscreen; positive N is exact rows; negative N is `terminal_rows + N`.

## Layout

- [`cmd/gitgum`](cmd/gitgum) — gitgum binary entry point
- [`cmd/fuzzyfinder`](cmd/fuzzyfinder) — `ff` binary entry point
- [`src/commands`](src/commands) — one file per subcommand, each implements `flags.Commander`
- [`src/fuzzyfinder`](src/fuzzyfinder) — picker library (originally a fork of `ktr0731/go-fuzzyfinder`, now substring-only matching and a custom renderer)
- [`src/litescreen`](src/litescreen) — standalone tcell-free ANSI renderer; powers inline (`--height`) mode
- [`internal/git`](internal/git) — git operations (the `Repo` type for parallel-safe tests, plus CWD-based free functions)
- [`internal/cmdrun`](internal/cmdrun) — small `exec.Command` wrappers
- [`internal/ui`](internal/ui) — picker helpers (`Select`, `Confirm`, `ErrCancelled`)
- [`internal/strutil`](internal/strutil) — string helpers
- [`internal/testutil/temp_repo`](internal/testutil/temp_repo) — test fixtures

## Known gaps

- **submodules**: not designed or tested with submodules in mind. behaviour is undefined when run inside a repo containing submodules — output may be incomplete or inconsistent. tracked as future work.

## Vendored licences

- [`src/fuzzyfinder/`](src/fuzzyfinder) derives from a vendored copy of `github.com/ktr0731/go-fuzzyfinder` (MIT). Original licence at [LICENSE-go-fuzzyfinder](LICENSE-go-fuzzyfinder).
- [`src/litescreen/`](src/litescreen) was developed based on `junegunn/fzf`'s `LightRenderer` (MIT). Code is freshly written, not copied; credit at [LICENSE-fzf](LICENSE-fzf).

## ToDo's

- [x] finish porting from bash version (what other commands do we want?)
- [x] integration tests with mock git repositories
- [x] ? unify switch
- [ ] make switch work well when offline
- [ ] de-novo shell completions
- [ ] redo `--fast`
- [x] display pages instead of rows