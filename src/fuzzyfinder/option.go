package fuzzyfinder

import (
	"context"

	"github.com/lczyk/gitgum/src/fuzzyfinder/matching"
)

type opt struct {
	mode         matching.Mode
	promptString string
	header       string
	ctx          context.Context
	query        string
	selectOne    bool
	matcher      func(query, item string) bool
}

const (
	// ModeSmart enables a smart matching. It is the default matching mode.
	// At the beginning, matching mode is ModeCaseInsensitive, but it switches
	// over to ModeCaseSensitive if an upper case character is inputted.
	ModeSmart = matching.ModeSmart
	// ModeCaseSensitive enables a case-sensitive matching.
	ModeCaseSensitive = matching.ModeCaseSensitive
	// ModeCaseInsensitive enables a case-insensitive matching.
	ModeCaseInsensitive = matching.ModeCaseInsensitive
)

var defaultOption = opt{
	promptString: "> ",
}

// Option represents available fuzzy-finding options.
type Option func(*opt)

// WithMode specifies a matching mode. The default mode is ModeSmart.
func WithMode(m matching.Mode) Option {
	return func(o *opt) {
		o.mode = m
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
