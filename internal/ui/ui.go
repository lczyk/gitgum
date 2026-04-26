package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/lczyk/gitgum/src/fuzzyfinder"
)

// ErrCancelled is returned when the user cancels a selection or confirmation (Ctrl+C or ESC).
var ErrCancelled = errors.New("cancelled")

// Select presents options via the fuzzyfinder library and returns the selected item.
func Select(prompt string, options []string, initialQuery ...string) (string, error) {
	return selectWith(fuzzyfinder.Find, prompt, options, initialQuery...)
}

func selectWith(finder func(context.Context, *[]string, sync.Locker, fuzzyfinder.Opt) ([]int, error), prompt string, options []string, initialQuery ...string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	opt := fuzzyfinder.Opt{Prompt: prompt + ": "}
	if len(initialQuery) > 0 {
		opt.Query = initialQuery[0]
	}

	idxs, err := finder(context.Background(), &options, nil, opt)
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("running picker: %w", err)
	}
	return options[idxs[0]], nil
}

func confirmWith(selector func(string, []string, ...string) (string, error), prompt string, defaultYes bool) (bool, error) {
	options := []string{"yes", "no"}
	if !defaultYes {
		options = []string{"no", "yes"}
	}
	selected, err := selector(prompt, options)
	if err != nil {
		return false, err
	}
	return selected == "yes", nil
}

// Confirm asks a yes/no question via the fuzzyfinder library.
func Confirm(prompt string, defaultYes bool) (bool, error) {
	return confirmWith(Select, prompt, defaultYes)
}
