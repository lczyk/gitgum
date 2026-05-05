// Package graph is a pure graph layout and rendering engine. It has no
// knowledge of git -- it takes abstract nodes with parent edges and produces
// ASCII graph output. The layout algorithm is a simplified column-assignment
// pass informed by git's graph.c, adapted for full-DAG (non-streaming) use.
package graph

// Node represents a vertex in the commit DAG. Parents is a forward edge list
// (this → parent). The graph engine builds reverse (child) edges internally.
type Node struct {
	ID         string // opaque identifier
	Label      string // display text appended after graph glyphs
	Parents    []string // parent IDs (empty for roots)
	Date       string   // sortable date string (ISO 8601) for ordering
	LayoutHint string   // optional hint for lane stability (empty = auto)
}

// Graph is the input to the layout engine.
type Graph struct {
	Nodes []Node
}

// Glyph is a single graph-drawing character in one column of one row.
type Glyph int

const (
	GlyphSpace     Glyph = iota // " "
	GlyphPipe                   // "|"
	GlyphStar                   // "*"
	GlyphSlash                  // "/"
	GlyphBackslash              // "\"
)

func (g Glyph) String() string {
	switch g {
	case GlyphSpace:
		return " "
	case GlyphPipe:
		return "|"
	case GlyphStar:
		return "*"
	case GlyphSlash:
		return "/"
	case GlyphBackslash:
		return "\\"
	}
	return " "
}

// GlyphKind classifies a piece of output text for the ColorScheme function.
type GlyphKind int

const (
	KindGraph   GlyphKind = iota // "|", "*", "/", "\"
	KindHash                     // abbreviated commit hash
	KindRef                      // "(HEAD -> main)"
	KindSubject                  // commit subject
)

// ColorScheme is called for each text fragment during rendering. Return the
// text unchanged if color is not desired, or wrapped in ANSI SGR escapes.
type ColorScheme func(kind GlyphKind, text string) string

// Row is one output line. If Commit is nil, this is a continuation row
// (only graph glyphs, no commit text).
type Row struct {
	Commit *Node   // nil on continuation rows
	Glyphs []Glyph // len == LayoutResult.Columns
}

// LayoutResult is the computed output of the layout engine.
type LayoutResult struct {
	Rows    []Row
	Columns int
}
