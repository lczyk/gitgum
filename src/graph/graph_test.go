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
		{ID: "a", Label: "a first commit", Parents: nil, Date: "2020-01-01T00:00:00Z"},
		{ID: "b", Label: "b second commit", Parents: []string{"a"}, Date: "2020-01-02T00:00:00Z"},
		{ID: "c", Label: "c third commit", Parents: []string{"b"}, Date: "2020-01-03T00:00:00Z"},
	}

	var e graph.Engine
	lr := e.Layout(graph.Graph{Nodes: nodes})
	lines := graph.Render(lr, nil)

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
		{ID: "a", Label: "a base", Parents: nil, Date: "2020-01-01T00:00:00Z"},
		{ID: "b", Label: "b branch1", Parents: []string{"a"}, Date: "2020-01-02T00:00:00Z"},
		{ID: "c", Label: "c branch2", Parents: []string{"a"}, Date: "2020-01-02T00:00:01Z"},
	}

	var e graph.Engine
	lr := e.Layout(graph.Graph{Nodes: nodes})
	lines := graph.Render(lr, nil)

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
		{ID: "a", Label: "a base", Parents: nil, Date: "2020-01-01T00:00:00Z"},
		{ID: "b", Label: "b main", Parents: []string{"a"}, Date: "2020-01-02T00:00:00Z"},
		{ID: "c", Label: "c feature", Parents: []string{"a"}, Date: "2020-01-02T00:00:01Z"},
		{ID: "d", Label: "d merge", Parents: []string{"b", "c"}, Date: "2020-01-03T00:00:00Z"},
	}

	var e graph.Engine
	lr := e.Layout(graph.Graph{Nodes: nodes})
	lines := graph.Render(lr, nil)

	// Should have 2 columns at the merge point.
	assert.Equal(t, lr.Columns, 2)

	assert.That(t, containsAny(lines, "a base"), "commit a")
	assert.That(t, containsAny(lines, "b main"), "commit b")
	assert.That(t, containsAny(lines, "c feature"), "commit c")
	assert.That(t, containsAny(lines, "d merge"), "commit d")

	// Merge should produce backslash or slash glyphs.
	assert.That(t, containsAny(lines, "\\") || containsAny(lines, "/"), "should have merge routing glyphs")
}

func TestRender_ColorScheme(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "a", Label: "abc1234 (HEAD -> main) hello world", Parents: nil, Date: "2020-01-01T00:00:00Z"},
	}

	cs := func(kind graph.GlyphKind, text string) string {
		switch kind {
		case graph.KindGraph:
			return "<g:" + text + ">"
		case graph.KindHash:
			return "<h:" + text + ">"
		case graph.KindRef:
			return "<r:" + text + ">"
		case graph.KindSubject:
			return "<s:" + text + ">"
		}
		return text
	}

	var e graph.Engine
	lr := e.Layout(graph.Graph{Nodes: nodes})
	lines := graph.Render(lr, cs)

	assert.Equal(t, len(lines), 1)
	expected := "<g:*><g: ><h:abc1234> <r:(HEAD -> main)> <s:hello world>"
	assert.Equal(t, lines[0], expected)
}

func TestRender_SingleNode(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "root", Label: "root initial", Parents: nil, Date: "2020-01-01T00:00:00Z"},
	}

	var e graph.Engine
	lr := e.Layout(graph.Graph{Nodes: nodes})
	lines := graph.Render(lr, nil)

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
