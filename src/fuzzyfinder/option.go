package fuzzyfinder

// Opt configures a fuzzy-finder run. The zero value is valid; only set the
// fields you want to override.
type Opt struct {
	// Prompt is the string drawn before the query. Defaults to "> ".
	Prompt string
	// Header is a static line displayed above the items.
	Header string
	// Query pre-fills the picker's query.
	Query string
	// SelectOne auto-selects the only matching item without showing the UI.
	SelectOne bool
	// Reverse renders the prompt at the top with items growing downward.
	// Default is bottom-up (prompt at bottom).
	Reverse bool
	// Multi lets the user select multiple items via Tab. When false, the
	// returned slice always has exactly one element.
	Multi bool
	// Height controls inline rendering.
	//   0   fullscreen (alt-screen)
	//   N>0 exactly N rows at the bottom; prior output preserved above
	//   N<0 terminal_rows + N
	// Honored only by the default litescreen renderer; ignored when the
	// FF_RENDERER=legacy escape hatch is set (tcell can't preserve
	// terminal scrollback).
	Height int
}

func (o Opt) withDefaults() Opt {
	if o.Prompt == "" {
		o.Prompt = "> "
	}
	return o
}
