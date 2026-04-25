package fuzzyfinder

import "context"

type opt struct {
	promptString string
	header       string
	ctx          context.Context
	query        string
	selectOne    bool
	reverse      bool
}

var defaultOption = opt{
	promptString: "> ",
}

// Option represents available fuzzy-finding options.
type Option func(*opt)

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

// WithQuery sets the initial query.
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

// WithReverse renders the prompt at the top and items growing downward.
// Default layout is bottom-up (prompt at bottom).
func WithReverse() Option {
	return func(o *opt) {
		o.reverse = true
	}
}

