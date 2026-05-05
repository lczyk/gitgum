package graph

import (
	"testing"

	"github.com/lczyk/assert"
)

// graph_internal_test.go holds white-box tests with access to the package's
// internal symbols. Black-box behavior tests live in graph_test.go.

func TestRender_Empty(t *testing.T) {
	t.Parallel()
	lr := Layout(nil)
	assert.Equal(t, len(lr.Rows), 0)
	assert.Equal(t, lr.Columns, 0)
	lines := Render(lr, Style{})
	assert.Equal(t, len(lines), 0)
}

func TestRender_TopologicalCorrection(t *testing.T) {
	t.Parallel()
	// Child dated before parent due to clock skew. Layout must correct.
	nodes := []Node{
		{ID: "parent", Label: "p parent", Parents: nil, Date: "2020-01-02T00:00:00Z"},
		{ID: "child", Label: "c child", Parents: []string{"parent"}, Date: "2020-01-01T00:00:00Z"},
	}

	lr := Layout(nodes)

	// Verify internal row assignment: parent must be at lower row than child.
	pState := findNode(lr, "parent")
	cState := findNode(lr, "child")
	assert.That(t, pState.row < cState.row, "parent row %d before child row %d", pState.row, cState.row)
}

// findNode locates a node's nodeState by ID for white-box assertions.
func findNode(lr LayoutResult, id string) struct{ row, col int } {
	for _, r := range lr.Rows {
		if r.Commit != nil && r.Commit.ID == id {
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
			return struct{ row, col int }{rowIndex(lr, r.Commit.ID), col}
		}
	}
	return struct{ row, col int }{-1, -1}
}

func rowIndex(lr LayoutResult, id string) int {
	idx := 0
	for _, r := range lr.Rows {
		if r.Commit != nil {
			if r.Commit.ID == id {
				return idx
			}
			idx++
		}
	}
	return -1
}
