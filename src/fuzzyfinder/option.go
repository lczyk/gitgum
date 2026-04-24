package fuzzyfinder

import (
	"context"
	"sync"
)

type opt struct {
	mode          mode
	hotReload     bool
	hotReloadLock sync.Locker
	promptString  string
	header        string
	ctx           context.Context
	query         string
	selectOne     bool
	matcher       func(query, item string) bool
}

type mode int

const (
	// ModeSmart enables a smart matching. It is the default matching mode.
	// At the beginning, matching mode is ModeCaseInsensitive, but it switches
	// over to ModeCaseSensitive if an upper case character is inputted.
	ModeSmart mode = iota
	// ModeCaseSensitive enables a case-sensitive matching.
	ModeCaseSensitive
	// ModeCaseInsensitive enables a case-insensitive matching.
	ModeCaseInsensitive
)

var defaultOption = opt{
	promptString:  "> ",
	hotReloadLock: &sync.Mutex{}, // avoid nil panic when hot reload is not used
}

// Option represents available fuzzy-finding options.
type Option func(*opt)

// WithMode specifies a matching mode. The default mode is ModeSmart.
func WithMode(m mode) Option {
	return func(o *opt) {
		o.mode = m
	}
}

// WithHotReloadLock reloads the passed slice automatically when some entries are appended.
// The caller must pass a pointer of the slice instead of the slice itself.
// The caller must pass a Locker which is used to synchronize access to the slice.
// The caller MUST NOT lock in the itemFunc passed to Find / FindMulti because it will be locked by the fuzzyfinder.
func WithHotReloadLock(lock sync.Locker) Option {
	return func(o *opt) {
		o.hotReload = true
		o.hotReloadLock = lock
	}
}

// WithPromptString changes the prompt string. The default value is "> ".
func WithPromptString(s string) Option {
	return func(o *opt) {
		o.promptString = s
	}
}

// WithHeader enables to set the header.
func WithHeader(s string) Option {
	return func(o *opt) {
		o.header = s
	}
}

// WithContext enables closing the fuzzy finder from parent.
func WithContext(ctx context.Context) Option {
	return func(o *opt) {
		o.ctx = ctx
	}
}

// WithQuery enables to set the initial query.
func WithQuery(s string) Option {
	return func(o *opt) {
		o.query = s
	}
}

// WithSelectOne automatically selects the item if there is only one match.
func WithSelectOne() Option {
	return func(o *opt) {
		o.selectOne = true
	}
}

// WithMatcher enables custom matching logic.
// The matcher function takes the current query and an item string, and returns true if the item matches.
// If not provided, the default fuzzy matching is used.
func WithMatcher(matcher func(query, item string) bool) Option {
	return func(o *opt) {
		o.matcher = matcher
	}
}
