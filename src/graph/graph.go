// Package graph is a pure graph layout and rendering engine. It has no
// knowledge of git -- it takes abstract nodes with parent edges and produces
// ASCII graph output. The layout algorithm is a simplified column-assignment
// pass informed by git's graph.c, adapted for full-DAG (non-streaming) use.
//
// Typical usage:
//
//	lr := graph.Layout(nodes)
//	lines := graph.Render(lr, graph.Style{}) // or pass a populated Style
package graph

// Node is a vertex in the commit DAG. Parents is a forward edge list
// (this -> parent). The engine builds reverse (child) edges internally.
//
// LayoutHint is an optional branch-name string used to keep tip commits on
// a stable col across calls. Two nodes carrying the same non-empty hint are
// pulled onto the same lane; mid-history nodes inherit from first-parent
// children regardless of hint. Empty disables hinting.
//
// Label is appended verbatim after the graph glyphs and a single space.
// Callers wanting per-segment coloring (hash, refs, subject) should embed
// ANSI escapes directly in Label -- the graph package does not parse it.
//
// IDs must be unique within a single Layout call -- duplicates panic.
// Cycles, self-parent edges, and parents that don't appear in the input
// are accepted but the resulting layout is unspecified (see edge tests
// for pinned behavior).
type Node struct {
	ID         string
	Label      string
	Parents    []string // parent IDs (empty for roots)
	Epoch      int64    // sort key (commonly unix epoch seconds; any monotonic int works). Optional -- when all Epochs are equal (incl. zero), nodes tiebreak by ID for deterministic layout.
	LayoutHint string
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

// String returns the single-character ASCII representation of g. Panics
// on values outside the iota range -- those represent internal corruption
// rather than user error.
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
	panic("graph: unknown Glyph value")
}

// Style controls the ANSI styling applied to graph glyphs. LinePrefix /
// LineSuffix wrap the line glyphs (`|`, `/`, `\`). StarPrefix / StarSuffix
// wrap commit markers (`*`). Spaces are written unwrapped.
//
// The zero Style produces plain ASCII output with no escapes.
type Style struct {
	LinePrefix, LineSuffix string
	StarPrefix, StarSuffix string
}

// Row is one output line. Commit is nil on stagger / continuation rows
// (graph glyphs only). Commit, when non-nil, points back into the input
// slice handed to Layout -- callers should not mutate the underlying
// Node through it.
type Row struct {
	Commit *Node
	Glyphs []Glyph // len == LayoutResult.Columns
}

// LayoutResult is the computed output of Layout. Rows is in oldest-first
// display order. Columns is the maximum lane index used.
type LayoutResult struct {
	Rows    []Row
	Columns int
}
