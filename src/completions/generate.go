package completions

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strings"
)

//go:embed gitgum.bash
var bashCompletion string

//go:embed gitgum.fish
var fishCompletion string

//go:embed gitgum.zsh
var zshCompletion string

const placeholder = "__GITGUM_CMD__"

// Render returns the completion script for the given shell with the command
// name substituted in.
func Render(shell, cmdName string) (string, error) {
	templates := map[string]string{
		"bash": bashCompletion,
		"fish": fishCompletion,
		"zsh":  zshCompletion,
	}
	content, ok := templates[shell]
	if !ok {
		shells := slices.Sorted(maps.Keys(templates))
		return "", fmt.Errorf("invalid shell type '%s', must be one of: %s", shell, strings.Join(shells, ", "))
	}
	return strings.ReplaceAll(content, placeholder, cmdName), nil
}
