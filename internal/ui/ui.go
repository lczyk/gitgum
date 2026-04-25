package ui

import (
	"errors"
	"fmt"

	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

// ErrCancelled is returned when the user cancels a picker operation (Ctrl+C or ESC).
var ErrCancelled = errors.New("picker operation cancelled")

// Select presents options via the fuzzyfinder library and returns the selected item.
func Select(prompt string, options []string, initialQuery ...string) (string, error) {
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

	idx, err := fuzzyfinder.Find(options, opts...)
	if err != nil {
		if err == fuzzyfinder.ErrAbort {
			return "", ErrCancelled
		}
		return "", err
	}
	return options[idx], nil
}

// Confirm asks a yes/no question via the fuzzyfinder library.
func Confirm(prompt string, defaultYes bool) (bool, error) {
	options := []string{"yes", "no"}
	if !defaultYes {
		options = []string{"no", "yes"}
	}
	selected, err := Select(prompt, options)
	if err != nil {
		return false, err
	}
	return selected == "yes", nil
}

// PrintHeader prints a dim header line.
func PrintHeader(message string) {
	fmt.Printf("\033[0;30m%s\033[0m\n", message)
}
