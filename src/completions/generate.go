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

// RenderFuzzyfinder returns the fuzzyfinder completion script for the given
// shell. (`ff` is just the install-time short name; the canonical binary is
// fuzzyfinder.)
func RenderFuzzyfinder(shell, cmdName string) (string, error) {
	return render(fuzzyfinderTemplates, shell, cmdName)
}
