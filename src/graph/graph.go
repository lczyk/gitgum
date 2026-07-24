// Package graph is a pure graph layout and rendering engine. It takes
// abstract nodes with parent edges and produces ASCII graph output in the
// style of version-control history graphs. The layout algorithm is a
// simplified column-assignment pass (historically informed by git's graph.c),
// adapted for full-DAG (non-streaming) use. The package itself has no notion
// of git: nodes, edges, lanes, and labels are the whole vocabulary.
//
// Typical usage:
//
//	lr := graph.Layout(nodes, graph.Opt{}) // or pass a populated Opt
//	lines := graph.Render(lr, graph.Style{}) // or pass a populated Style
package graph

// Node is a vertex in the DAG. Parents is a forward edge list
// (this -> parent). The engine builds reverse (child) edges internally.
// Parents[0] is the primary edge: layout keeps its lane continuous through
// the node, so chains linked by primary edges render as one column.
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
	ID      string
	Label   string
	Parents []string // parent IDs (empty for roots)
	Epoch   int64    // sort key (commonly unix epoch seconds; any monotonic int works). Optional -- when all Epochs are equal (incl. zero), nodes tiebreak by ID for deterministic layout.
	// Float sinks this node toward the bottom of the output: the node and its
	// descendant-closure sort ahead of everything else, so it lands as low as
	// the DAG allows -- exactly the bottom row when it's a tip. Any number of
	// nodes may set it; the union of their closures floats as one partition,
	// date-ordered within. Note the closure expansion: flagging a node floats
	// everything below it too. Unset on every node = no-op.
	Float bool
}

// Glyph is a single graph-drawing character in one column of one row.
type Glyph int

const (
	GlyphSpace     Glyph = iota // " "
	GlyphPipe                   // "|"
	GlyphStar                   // "*"
	GlyphSlash                  // "/"
	GlyphBackslash              // "\"
	// GlyphV marks the point where an edge leaves a lane and no node sits
	// there -- the arrowhead stands in for the `*` a node would have drawn.
	GlyphV // "v"
	// GlyphCaret is GlyphV's vertical mirror, used on rows drawn newest-first.
	GlyphCaret // "^"
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
	case GlyphV:
		return "v"
	case GlyphCaret:
		return "^"
	}
	panic("graph: unknown Glyph value")
}

// mirror returns g reflected across a horizontal axis -- the glyph that draws
// the same edge once row order is flipped. Diagonals swap handedness and the
// split marker flips its arrowhead; the rest are symmetric.
func mirror(g Glyph) Glyph {
	switch g {
	case GlyphSlash:
		return GlyphBackslash
	case GlyphBackslash:
		return GlyphSlash
	case GlyphV:
		return GlyphCaret
	case GlyphCaret:
		return GlyphV
	}
	return g
}

// isSplitMark reports whether g is one of the two split-point arrowheads.
func isSplitMark(g Glyph) bool { return g == GlyphV || g == GlyphCaret }

// Style controls the ANSI styling applied to graph glyphs. LinePrefix /
// LineSuffix wrap the line glyphs (`|`, `/`, `\`). StarPrefix / StarSuffix
// wrap node markers (`*`). Spaces are written unwrapped.
//
// The zero Style produces plain ASCII output with no escapes.
type Style struct {
	LinePrefix, LineSuffix string
	StarPrefix, StarSuffix string
}

// Row is one output line. Node is nil on stagger / continuation rows
// (graph glyphs only). Node, when non-nil, points back into the input
// slice handed to Layout -- callers should not mutate the underlying
// Node through it.
type Row struct {
	Node   *Node
	Glyphs []Glyph // len == LayoutResult.Columns
	// Gap, when non-nil, holds the glyph drawn in each column's trailing
	// half-slot (the space to its right). Diagonals climb half a column per
	// row, so they alternate between the two kinds of slot: on the gap rows
	// the diagonal lands in Gap[c] and the pipes at columns c and c+1 both
	// stay intact (`|\|` weave); on the rows in between it takes a column's
	// primary slot in Glyphs instead. That second case is a genuine crossing
	// -- if the column holds a live lane, the diagonal replaces its pipe for
	// that one row, which is the intended look and not a layout fault. Space
	// (zero value) means an ordinary column gap.
	Gap []Glyph
}

// LayoutResult is the computed output of Layout. Rows is in oldest-first
// display order, or newest-first under Opt.Reverse. Columns is the number of
// columns the layout needed -- the width of every Row's Glyphs slice, one past
// the highest column index any lane occupied.
type LayoutResult struct {
	Rows    []Row
	Columns int
}
