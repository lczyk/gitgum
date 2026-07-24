package graph_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/graph"
)

// Node.Float sinks the flagged node + its descendants to the bottom of the
// oldest-first output, so it lands as low as the DAG allows.

func TestFloat_TipSinksToBottom(t *testing.T) {
	t.Parallel()
	// base forks into x (older tip, floated) and y (newer tip). Without
	// floating the newer tip y would be the bottom row; Float pins x there.
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "x", Label: "x floated", Parents: []string{"base"}, Epoch: 5, Float: true},
		{ID: "y", Label: "y other", Parents: []string{"base"}, Epoch: 10},
	}
	lines := graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{})
	assert.Equal(t, indexOf(lines, "x floated"), len(lines)-1) // tip -> exact bottom
	assert.That(t, indexOf(lines, "y other") < indexOf(lines, "x floated"), "newer tip floats above the flagged node")
}

func TestFloat_MidGraphAsLowAsPossible(t *testing.T) {
	t.Parallel()
	// f is floated with a descendant `child`; `other` is an unrelated newer tip.
	// child must stay below f (it's a descendant), but both sink below `other`.
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "f", Label: "f floated", Parents: []string{"base"}, Epoch: 5, Float: true},
		{ID: "child", Label: "child of floated", Parents: []string{"f"}, Epoch: 6},
		{ID: "other", Label: "other chain", Parents: []string{"base"}, Epoch: 100},
	}
	lines := graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{})
	assert.Equal(t, indexOf(lines, "child of floated"), len(lines)-1) // float-side tip -> bottom
	assert.That(t, indexOf(lines, "f floated") < indexOf(lines, "child of floated"), "child stays below the flagged node")
	assert.That(t, indexOf(lines, "other chain") < indexOf(lines, "f floated"), "unrelated newer chain floats above the flagged lineage")
}

func TestFloat_NoFlagUnchanged(t *testing.T) {
	t.Parallel()
	// No Float set: ordering is the plain newest-at-bottom default.
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "x", Label: "x older", Parents: []string{"base"}, Epoch: 5},
		{ID: "y", Label: "y newer", Parents: []string{"base"}, Epoch: 10},
	}
	lines := graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{})
	assert.Equal(t, indexOf(lines, "y newer"), len(lines)-1) // newest at bottom
}

func TestFloat_MultipleSeeds(t *testing.T) {
	t.Parallel()
	// Two floated tips on divergent chains: both sink below the unflagged
	// newer tip, and order between them falls back to date (newest lowest).
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "a", Label: "a floated", Parents: []string{"base"}, Epoch: 5, Float: true},
		{ID: "b", Label: "b floated", Parents: []string{"base"}, Epoch: 7, Float: true},
		{ID: "top", Label: "top other", Parents: []string{"base"}, Epoch: 100},
	}
	lines := graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{})
	assert.That(t, indexOf(lines, "top other") < indexOf(lines, "a floated"), "unflagged tip stays above the floated partition")
	assert.That(t, indexOf(lines, "top other") < indexOf(lines, "b floated"), "unflagged tip stays above the floated partition")
	assert.Equal(t, indexOf(lines, "b floated"), len(lines)-1) // newest floated tip takes the bottom row
	assert.That(t, indexOf(lines, "a floated") < indexOf(lines, "b floated"), "floated tips date-order within the partition")
}
