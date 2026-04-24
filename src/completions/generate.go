package completions

import _ "embed"

//go:embed gitgum.bash
var bashCompletion string

//go:embed gitgum.fish
var fishCompletion string

//go:embed gitgum.zsh
var zshCompletion string

// CompletionTemplates contains the embedded shell completion scripts
var CompletionTemplates = map[string]string{
	"bash": bashCompletion,
	"fish": fishCompletion,
	"zsh":  zshCompletion,
}
