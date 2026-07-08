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

func selectWith(finder func(context.Context, *[]string, sync.Locker, ff.Opt) (ff.Result, error), height int, prompt string, options []string, initialQuery ...string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	opt := ff.Opt{Prompt: prompt + ": ", Height: height, Reverse: true}
	if len(initialQuery) > 0 {
		opt.Query = initialQuery[0]
	}

	res, err := finder(context.Background(), &options, nil, opt)
	if err != nil {
		if errors.Is(err, ff.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("running picker: %w", err)
	}
	// Enter with nothing selectable under the query. These pickers offer no
	// other way out, so treat it as the user backing out.
	if len(res.Indices) == 0 {
		return "", ErrCancelled
	}
	return options[res.Indices[0]], nil
}

// SelectStream is like Select but reads candidates from a SliceSource that
// may grow (or shrink) concurrently — used by callers that stream entries
// from background goroutines (e.g. switch). ctx is cancelled when the
// consumer is done; that also tells producers to stop.
func SelectStream(ctx context.Context, prompt string, src *ff.SliceSource, unselectable func(string) bool) (string, error) {
	opt := ff.Opt{Prompt: prompt + ": ", Height: 10, Reverse: true, Unselectable: unselectable}
	res, err := ff.FindFromSource(ctx, src, opt)
	if err != nil {
		if errors.Is(err, ff.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("running picker: %w", err)
	}
	// Enter with nothing selectable under the query (e.g. every branch is
	// checked out in another worktree). Nothing to hand back, so it reads as
	// a cancellation -- callers already print their own "nothing selected".
	if len(res.Items) == 0 {
		return "", ErrCancelled
	}
	return res.Items[0], nil
}

// Prompt reads a line of free text via the picker. The item list is empty, so
// nothing is ever selectable and Enter can only mean "take what I typed" -- the
// empty-selection case ff.Result documents. What comes back is the query
// verbatim, "" included; validating it is the caller's job.
//
// Returns ErrCancelled on Esc/Ctrl-C.
func Prompt(question string) (string, error) {
	src := ff.NewSliceSourceFrom(nil)
	opt := ff.Opt{Prompt: question + ": ", Height: 1, Reverse: true}
	res, err := ff.FindFromSource(context.Background(), src, opt)
	if err != nil {
		if errors.Is(err, ff.ErrAbort) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("running picker: %w", err)
	}
	return res.Query, nil
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
	SelectStream(ctx context.Context, prompt string, src *ff.SliceSource, unselectable func(string) bool) (string, error)
	MultiSelect(prompt string, options []string) ([]string, error)
	Prompt(question string) (string, error)
	Confirm(prompt string, defaultYes bool) (bool, error)
}

// MultiSelect presents options via the fuzzyfinder library with multi-select
// enabled (Tab to mark, Enter to confirm). Returns the selected items in
// selection order, or ErrCancelled if the user aborts (Esc/Ctrl+C).
func MultiSelect(prompt string, options []string) ([]string, error) {
	if len(options) == 0 {
		return nil, fmt.Errorf("no options provided")
	}
	height := min(10, len(options))
	opt := ff.Opt{Prompt: prompt + ": ", Height: height, Reverse: true, Multi: true}
	res, err := ff.Find(context.Background(), &options, nil, opt)
	if err != nil {
		if errors.Is(err, ff.ErrAbort) {
			return nil, ErrCancelled
		}
		return nil, fmt.Errorf("running picker: %w", err)
	}
	// Enter with nothing matched marks nothing; same as backing out.
	if len(res.Indices) == 0 {
		return nil, ErrCancelled
	}
	out := make([]string, len(res.Indices))
	for i, idx := range res.Indices {
		out[i] = options[idx]
	}
	return out, nil
}

// RealSelector is the production Selector. Methods delegate to ui.Select,
// ui.SelectStream, and ui.Confirm, which drive the real fuzzyfinder UI.
type RealSelector struct{}

func (RealSelector) Select(prompt string, options []string, initialQuery ...string) (string, error) {
	return Select(prompt, options, initialQuery...)
}

func (RealSelector) SelectStream(ctx context.Context, prompt string, src *ff.SliceSource, unselectable func(string) bool) (string, error) {
	return SelectStream(ctx, prompt, src, unselectable)
}

func (RealSelector) MultiSelect(prompt string, options []string) ([]string, error) {
	return MultiSelect(prompt, options)
}

func (RealSelector) Prompt(question string) (string, error) {
	return Prompt(question)
}

func (RealSelector) Confirm(prompt string, defaultYes bool) (bool, error) {
	return Confirm(prompt, defaultYes)
}
