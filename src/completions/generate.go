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

var templates = map[string]string{
	"bash": bashCompletion,
	"fish": fishCompletion,
	"zsh":  zshCompletion,
}

func Get(shell string) (string, error) {
	content, ok := templates[shell]
	if !ok {
		shells := slices.Sorted(maps.Keys(templates))
		return "", fmt.Errorf("invalid shell type '%s', must be one of: %s", shell, strings.Join(shells, ", "))
	}
	return content, nil
}
