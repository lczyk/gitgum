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

## ToDo's

- [ ] finish porting from bash version (what other commands do we want?)
- [x] integration tests with mock git repositories
- [ ] ? unify switch
- [ ] make switch work well when offline
- [ ] de-novo shell completions

