package graph

// Opt configures a Layout call. The zero value is valid; only set the fields
// you want to override.
//
// Every field here is a layout-time concern. Glyph styling is a render-time
// one and lives in Style, which Render takes separately.
//
// Opt holds no func fields on purpose: Layout is a pure function of (nodes,
// opt), and keeping Opt comparable leaves the door open to caching on it.
type Opt struct {
	// Reverse emits rows newest-first instead of oldest-first, mirroring every
	// glyph so each edge still points along itself: `/` becomes `\`, `v`
	// becomes `^`.
	Reverse bool
	// NoSplitMarks draws a plain pipe where a lane's edge parts from it,
	// instead of the `v` (or `^` under Reverse) that marks the split.
	// (With marks off the output matches classic `git log --graph` style.)
	NoSplitMarks bool
}
