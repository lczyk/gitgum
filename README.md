# gitgum

A bunch of git commands with [`fzf`](https://github.com/junegunn/fzf) interface (used to be [gum](https://github.com/charmbracelet/gum), hence the name). The features are very tailored to the kind of workflows I have, but nothing work-specific is encoded in.

## Build

```bash
make
```

## Install

```bash
ln -s "$PWD/bin/gitgum" ~/.local/bin/gitgum
# Or use an alias like 'gg':
ln -s "$PWD/bin/gitgum" ~/.local/bin/gg
```

## Shell completions

Generate shell completions:

```bash
# Fish
gitgum completion fish | source

# Bash
eval "$(gitgum completion bash)"

# Zsh
eval "$(gitgum completion zsh)"
```

## Commands

### `gitgum delete`

Interactively delete a local branch and optionally its remote tracking branch. The command will:
- Let you select a branch to delete using fzf
- Warn before deleting `main` or `master` branches
- Prompt to switch branches if you're trying to delete the current branch
- Detect remote tracking branches and ask whether to delete them
- Attempt a safe delete first (`git branch -d`), falling back to force delete (`git branch -D`) with confirmation

### `gitgum replay-list A B`

List commits on branch A that have diverged from trunk/base branch B, in chronological order. This is equivalent to running:
```bash
git rev-list $(git merge-base A B)..A --reverse
```

Useful for identifying the feature commits on a branch that need to be replayed or cherry-picked onto another base.

## ToDo's

- [ ] finish porting from bash version (what other commands do we want?)
- [x] integration tests with mock git repositories
- [ ] ? unify switch
- [ ] make switch work well when offline
- [ ] de-novo shell completions

