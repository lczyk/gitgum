package ui

import (
	"errors"
	"fmt"

	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

// ErrFzfCancelled is returned when the user cancels an fzf operation (Ctrl+C or ESC).
var ErrFzfCancelled = errors.New("fzf operation cancelled")

// FzfSelect presents options via fzf and returns the selected item.
func FzfSelect(prompt string, options []string, initialQuery ...string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	opts := []fuzzyfinder.Option{
		fuzzyfinder.WithPromptString(prompt + ": "),
	}
	if len(initialQuery) > 0 {
		opts = append(opts, fuzzyfinder.WithQuery(initialQuery[0]))
	}
	opts = append(opts, fuzzyfinder.WithMatcher(fuzzyfinder.SubstringMatcher))

	idx, err := fuzzyfinder.Find(
		options,
		func(i int) string { return options[i] },
		opts...,
	)
	if err != nil {
		if err == fuzzyfinder.ErrAbort {
			return "", ErrFzfCancelled
		}
		return "", err
	}
	return options[idx], nil
}

// FzfConfirm asks a yes/no question via fzf.
func FzfConfirm(prompt string, defaultYes bool) (bool, error) {
	options := []string{"yes", "no"}
	if !defaultYes {
		options = []string{"no", "yes"}
	}
	selected, err := FzfSelect(prompt, options)
	if err != nil {
		return false, err
	}
	return selected == "yes", nil
}

// PrintHeader prints a dim header line.
func PrintHeader(message string) {
	fmt.Printf("\033[0;30m%s\033[0m\n", message)
}
