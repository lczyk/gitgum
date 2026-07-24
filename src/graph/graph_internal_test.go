package graph

import (
	"testing"

	"github.com/lczyk/assert"
)

// graph_internal_test.go holds white-box tests with access to the package's
// internal symbols. Black-box behavior tests live in graph_test.go.

func TestRender_Empty(t *testing.T) {
	t.Parallel()
	lr := Layout(nil, Opt{})
	assert.Equal(t, len(lr.Rows), 0)
	assert.Equal(t, lr.Columns, 0)
	lines := Render(lr, Style{})
	assert.Equal(t, len(lines), 0)
}

func TestRender_TopologicalCorrection(t *testing.T) {
	t.Parallel()
	// Child dated before parent due to clock skew. Layout must correct.
	nodes := []Node{
		{ID: "parent", Label: "p parent", Parents: nil, Epoch: 100},
		{ID: "child", Label: "c child", Parents: []string{"parent"}, Epoch: 0},
	}

	lr := Layout(nodes, Opt{})

	// Verify internal row assignment: parent must be at lower row than child.
	pState := findNode(lr, "parent")
	cState := findNode(lr, "child")
	assert.That(t, pState.row < cState.row, "parent row %d before child row %d", pState.row, cState.row)
}

func TestRender_GapDiagonalPastLastPipe(t *testing.T) {
	t.Parallel()
	// A crossing diagonal can land in a trailing gap slot (right of column c)
	// with no pipe at column c and nothing active further right -- an edge
	// routed across an empty column, or a birth diagonal over a not-yet-settled
	// lane. The right edge must extend to cover that gap glyph; counting only
	// primary column glyphs truncates it, dropping the diagonal and leaving a
	// bare pipe row.
	lr := LayoutResult{
		Columns: 3,
		Rows: []Row{{
			Glyphs: []Glyph{GlyphPipe, GlyphSpace, GlyphSpace},
			Gap:    []Glyph{GlyphSpace, GlyphBackslash, GlyphSpace},
		}},
	}
	lines := Render(lr, Style{})
	assert.Equal(t, len(lines), 1)
	assert.Equal(t, lines[0], "|  \\")
}

// findNode locates a node's nodeState by ID for white-box assertions.
func findNode(lr LayoutResult, id string) struct{ row, col int } {
	for _, r := range lr.Rows {
		if r.Node != nil && r.Node.ID == id {
			// Find col by scanning glyphs for GlyphStar.
			col := -1
			for c, g := range r.Glyphs {
				if g == GlyphStar {
					col = c
					break
				}
			}
			// Row index in Rows is not the engine row number; recover from
			// position in Rows array (commit rows preserve oldest-first order).
			return struct{ row, col int }{rowIndex(lr, r.Node.ID), col}
		}
	}
	return struct{ row, col int }{-1, -1}
}

func rowIndex(lr LayoutResult, id string) int {
	idx := 0
	for _, r := range lr.Rows {
		if r.Node != nil {
			if r.Node.ID == id {
				return idx
			}
			idx++
		}
	}
	return -1
}
