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

//go:embed gitgum.nu
var nuCompletion string

const Placeholder = "__GITGUM_CMD__"

var templates = map[string]string{
	"bash": bashCompletion,
	"fish": fishCompletion,
	"zsh":  zshCompletion,
	"nu":   nuCompletion,
}

func Render(shell, cmdName string) (string, error) {
	content, ok := templates[shell]
	if !ok {
		return "", fmt.Errorf("invalid shell type '%s', must be one of: bash, fish, zsh, nu", shell)
	}
	return strings.ReplaceAll(content, Placeholder, cmdName), nil
}
