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
	// Height controls inline rendering. Counts item rows only — the prompt
	// and number-line (and header, when set) are drawn in addition.
	//   0   fullscreen (alt-screen)
	//   N>0 N visible item rows; prompt+number-line+header drawn above/below
	//   N<0 terminal_rows + N (raw band size, unchanged from prior behavior)
	// Honored only by the default litescreen renderer; ignored when the
	// FF_RENDERER=legacy escape hatch is set (tcell can't preserve
	// terminal scrollback).
	Height int
	// RedrawAggressive forces a full repaint on every draw instead of the
	// O(diff) emit. Set this when a sibling process may write to the same
	// terminal as the picker (e.g. `find ~ | fuzzyfinder`, where find's
	// stderr "permission denied" lines tear the picker). Cost is a few KB
	// of ANSI per frame; safe to leave off otherwise.
	RedrawAggressive bool
	// Ansi treats item strings as carrying ANSI SGR escape sequences.
	// Items are parsed once, drawn with their native style, and stripped
	// for matching. The cursor and search-highlight overrides still apply
	// (and win) over the per-rune ansi style. When false (default), items
	// are drawn as plain text and any escapes render as literal cells.
	Ansi bool
}

func (o Opt) withDefaults() Opt {
	if o.Prompt == "" {
		o.Prompt = "> "
	}
	return o
}
