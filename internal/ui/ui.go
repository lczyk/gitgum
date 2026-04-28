package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ff "github.com/lczyk/gitgum/src/fuzzyfinder"
)


// ErrCancelled is returned when the user cancels a selection or confirmation (Ctrl+C or ESC).
var ErrCancelled = errors.New("cancelled")

// Select presents options via the fuzzyfinder library and returns the selected item.
func Select(prompt string, options []string, initialQuery ...string) (string, error) {
	return selectWith(ff.Find, 10, prompt, options, initialQuery...)
}

func selectShort(prompt string, options []string, initialQuery ...string) (string, error) {
	return selectWith(ff.Find, 2, prompt, options, initialQuery...)
}

func selectWith(finder func(context.Context, *[]string, sync.Locker, ff.Opt) ([]int, error), height int, prompt string, options []string, initialQuery ...string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	opt := ff.Opt{Prompt: prompt + ": ", Height: height, Reverse: true}
	if len(initialQuery) > 0 {
		opt.Query = initialQuery[0]
	}

	idxs, err := finder(context.Background(), &options, nil, opt)
	if err != nil {
		if errors.Is(err, ff.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("running picker: %w", err)
	}
	return options[idxs[0]], nil
}

// SelectStream is like Select but reads candidates from a SliceSource that
// may grow (or shrink) concurrently — used by callers that stream entries
// from background goroutines (e.g. switch). ctx is cancelled when the
// consumer is done; that also tells producers to stop.
func SelectStream(ctx context.Context, prompt string, src *ff.SliceSource) (string, error) {
	opt := ff.Opt{Prompt: prompt + ": ", Height: 10, Reverse: true}
	selected, err := ff.FindFromSource(ctx, src, opt)
	if err != nil {
		if errors.Is(err, ff.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("running picker: %w", err)
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("no selection returned")
	}
	return selected[0], nil
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
	return confirmWith(selectShort, prompt, defaultYes)
}

// Selector is the interactive-input surface commands use. Tests inject a stub
// to drive selections deterministically without a TTY; production code uses
// RealSelector (the zero value), which delegates to the package-level functions.
type Selector interface {
	Select(prompt string, options []string, initialQuery ...string) (string, error)
	SelectStream(ctx context.Context, prompt string, src *ff.SliceSource) (string, error)
	Confirm(prompt string, defaultYes bool) (bool, error)
}

// RealSelector is the production Selector. Methods delegate to ui.Select,
// ui.SelectStream, and ui.Confirm, which drive the real fuzzyfinder UI.
type RealSelector struct{}

func (RealSelector) Select(prompt string, options []string, initialQuery ...string) (string, error) {
	return Select(prompt, options, initialQuery...)
}

func (RealSelector) SelectStream(ctx context.Context, prompt string, src *ff.SliceSource) (string, error) {
	return SelectStream(ctx, prompt, src)
}

func (RealSelector) Confirm(prompt string, defaultYes bool) (bool, error) {
	return Confirm(prompt, defaultYes)
}
