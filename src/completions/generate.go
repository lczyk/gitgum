package completions

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed gitgum.bash
var gitgumBash string

//go:embed gitgum.fish
var gitgumFish string

//go:embed gitgum.zsh
var gitgumZsh string

//go:embed gitgum.nu
var gitgumNu string

//go:embed cd.bash
var cdBash string

//go:embed cd.fish
var cdFish string

//go:embed cd.zsh
var cdZsh string

//go:embed cd.nu
var cdNu string

//go:embed fuzzyfinder.bash
var fuzzyfinderBash string

//go:embed fuzzyfinder.fish
var fuzzyfinderFish string

//go:embed fuzzyfinder.zsh
var fuzzyfinderZsh string

//go:embed fuzzyfinder.nu
var fuzzyfinderNu string

const Placeholder = "__GITGUM_CMD__"

var gitgumTemplates = map[string]string{
	"bash": gitgumBash,
	"fish": gitgumFish,
	"zsh":  gitgumZsh,
	"nu":   gitgumNu,
}

var cdTemplates = map[string]string{
	"bash": cdBash,
	"fish": cdFish,
	"zsh":  cdZsh,
	"nu":   cdNu,
}

var fuzzyfinderTemplates = map[string]string{
	"bash": fuzzyfinderBash,
	"fish": fuzzyfinderFish,
	"zsh":  fuzzyfinderZsh,
	"nu":   fuzzyfinderNu,
}

func render(templates map[string]string, shell, cmdName string) (string, error) {
	content, ok := templates[shell]
	if !ok {
		return "", fmt.Errorf("invalid shell type '%s', must be one of: bash, fish, zsh, nu", shell)
	}
	return strings.ReplaceAll(content, Placeholder, cmdName), nil
}

// Render returns the gitgum completion script for the given shell with cmdName
// substituted in place of the placeholder.
func Render(shell, cmdName string) (string, error) {
	return render(gitgumTemplates, shell, cmdName)
}

// RenderCd returns the shell function that makes `<cmdName> worktree-switch`
// (and its `w` alias) change the shell's directory. It is kept apart from the
// completion script so sourcing completions never redefines the command.
func RenderCd(shell, cmdName string) (string, error) {
	return render(cdTemplates, shell, cmdName)
}

// RenderFuzzyfinder returns the fuzzyfinder completion script for the given
// shell. (`ff` is just the install-time short name; the canonical binary is
// fuzzyfinder.)
func RenderFuzzyfinder(shell, cmdName string) (string, error) {
	return render(fuzzyfinderTemplates, shell, cmdName)
}
