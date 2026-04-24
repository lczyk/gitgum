package completions

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed gitgum.bash
var bashCompletion string

//go:embed gitgum.fish
var fishCompletion string

//go:embed gitgum.zsh
var zshCompletion string

const placeholder = "__GITGUM_CMD__"

var templates = map[string]string{
	"bash": bashCompletion,
	"fish": fishCompletion,
	"zsh":  zshCompletion,
}

var validShells = []string{"bash", "fish", "zsh"}

// Render returns the completion script for the given shell with the command
// name substituted in.
func Render(shell, cmdName string) (string, error) {
	content, ok := templates[shell]
	if !ok {
		return "", fmt.Errorf("invalid shell type '%s', must be one of: %s", shell, strings.Join(validShells, ", "))
	}
	return strings.ReplaceAll(content, placeholder, cmdName), nil
}
