package graph_test

import (
	"strings"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/graph"
)

// graph_test.go holds black-box tests using only the graph package's public
// API. White-box tests with access to internals live in graph_internal_test.go.

func TestRender_Linear(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "a", Label: "a first commit", Parents: nil, Epoch: 1},
		{ID: "b", Label: "b second commit", Parents: []string{"a"}, Epoch: 2},
		{ID: "c", Label: "c third commit", Parents: []string{"b"}, Epoch: 200},
	}

	lr := graph.Layout(nodes)
	lines := graph.Render(lr, graph.Style{})

	// Oldest first: a, then b, then c. No branching, so all in column 0.
	assert.Equal(t, lr.Columns, 1)
	assert.Equal(t, len(lines), 3)

	// Each line should be "* <label>" with no graph prefix beyond the star.
	for i, line := range lines {
		assert.That(t, strings.HasPrefix(line, "* "), "line %d should start with '* '", i)
	}
	assert.That(t, strings.Contains(lines[0], "a first commit"), "first line should be commit a")
	assert.That(t, strings.Contains(lines[1], "b second commit"), "second line should be commit b")
	assert.That(t, strings.Contains(lines[2], "c third commit"), "third line should be commit c")

	// Ordering: a before b before c.
	idxA := indexOf(lines, "a first commit")
	idxB := indexOf(lines, "b second commit")
	idxC := indexOf(lines, "c third commit")
	assert.That(t, idxA < idxB, "a before b")
	assert.That(t, idxB < idxC, "b before c")
}

func TestRender_Fork(t *testing.T) {
	t.Parallel()
	//   b
	//  /
	// a
	//  \
	//   c
	nodes := []graph.Node{
		{ID: "a", Label: "a base", Parents: nil, Epoch: 1},
		{ID: "b", Label: "b branch1", Parents: []string{"a"}, Epoch: 2},
		{ID: "c", Label: "c branch2", Parents: []string{"a"}, Epoch: 101},
	}

	lr := graph.Layout(nodes)
	lines := graph.Render(lr, graph.Style{})

	// Two columns: main branch (a→b) in col 0, branch2 (c) in col 1.
	assert.Equal(t, lr.Columns, 2)

	assert.That(t, strings.Contains(lines[0], "a base"), "commit a visible")

	// Both b and c should be present.
	assert.That(t, containsAny(lines, "b branch1"), "commit b visible")
	assert.That(t, containsAny(lines, "c branch2"), "commit c visible")

	// Fork should produce graph glyphs.
	assert.That(t, containsAny(lines, "|"), "should have pipe glyphs for fork")
}

func TestRender_Merge(t *testing.T) {
	t.Parallel()
	// a -- b -- d (merge)
	//   \-- c -/
	nodes := []graph.Node{
		{ID: "a", Label: "a base", Parents: nil, Epoch: 1},
		{ID: "b", Label: "b main", Parents: []string{"a"}, Epoch: 2},
		{ID: "c", Label: "c feature", Parents: []string{"a"}, Epoch: 101},
		{ID: "d", Label: "d merge", Parents: []string{"b", "c"}, Epoch: 200},
	}

	lr := graph.Layout(nodes)
	lines := graph.Render(lr, graph.Style{})

	// Should have 2 columns at the merge point.
	assert.Equal(t, lr.Columns, 2)

	assert.That(t, containsAny(lines, "a base"), "commit a")
	assert.That(t, containsAny(lines, "b main"), "commit b")
	assert.That(t, containsAny(lines, "c feature"), "commit c")
	assert.That(t, containsAny(lines, "d merge"), "commit d")

	// Merge should produce backslash or slash glyphs.
	assert.That(t, containsAny(lines, "\\") || containsAny(lines, "/"), "should have merge routing glyphs")
}

func TestRender_Style(t *testing.T) {
	t.Parallel()
	// Style wraps the line/star glyphs; labels are emitted opaque (caller
	// is expected to embed any per-segment ANSI before Layout).
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "side", Label: "side", Parents: []string{"base"}, Epoch: 1, Lane: h("side")},
		{ID: "main", Label: "main", Parents: []string{"base", "side"}, Epoch: 2, Lane: h("main")},
	}
	st := graph.Style{
		LinePrefix: "<L>", LineSuffix: "</L>",
		StarPrefix: "<S>", StarSuffix: "</S>",
	}
	lr := graph.Layout(nodes)
	lines := graph.Render(lr, st)
	assert.That(t, len(lines) >= 3, "at least 3 rows produced")
	// First row: "* base" -> star wrapped, then ' base'.
	assert.Equal(t, lines[0], "<S>*</S> base")
	// Stagger row contains a wrapped line glyph (`|\`-ish).
	stagger := lines[1]
	assert.That(t, strings.Contains(stagger, "<L>"), "stagger row uses LinePrefix")
}

func TestRender_SingleNode(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "root", Label: "root initial", Parents: nil, Epoch: 1},
	}

	lr := graph.Layout(nodes)
	lines := graph.Render(lr, graph.Style{})

	assert.Equal(t, len(lines), 1)
	assert.Equal(t, lr.Columns, 1)
	assert.That(t, strings.HasPrefix(lines[0], "* root initial"), "single root commit")
}

// ------ helpers ----------------------------------------------------------

func indexOf(lines []string, substr string) int {
	for i, line := range lines {
		if strings.Contains(line, substr) {
			return i
		}
	}
	return -1
}

func containsAny(lines []string, substr string) bool {
	return indexOf(lines, substr) >= 0
}
