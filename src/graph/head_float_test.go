package graph_test

import (
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/graph"
)

// IsHead floats the checked-out commit + its descendants to the bottom of the
// oldest-first output, so HEAD lands as low as the DAG allows.

func TestHeadFloat_TipSinksToBottom(t *testing.T) {
	t.Parallel()
	// base forks into x (older tip, HEAD) and y (newer tip). Without floating
	// the newer tip y would be the bottom row; HEAD pins x there instead.
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "x", Label: "x head", Parents: []string{"base"}, Epoch: 5, IsHead: true},
		{ID: "y", Label: "y other", Parents: []string{"base"}, Epoch: 10},
	}
	lines := graph.Render(graph.Layout(nodes), graph.Style{})
	assert.Equal(t, indexOf(lines, "x head"), len(lines)-1) // tip -> exact bottom
	assert.That(t, indexOf(lines, "y other") < indexOf(lines, "x head"), "newer branch floats above HEAD")
}

func TestHeadFloat_MidHistoryAsLowAsPossible(t *testing.T) {
	t.Parallel()
	// h is HEAD with a descendant `child`; `other` is an unrelated newer branch.
	// child must stay below h (it's a descendant), but both sink below `other`.
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "h", Label: "h head", Parents: []string{"base"}, Epoch: 5, IsHead: true},
		{ID: "child", Label: "child of head", Parents: []string{"h"}, Epoch: 6},
		{ID: "other", Label: "other branch", Parents: []string{"base"}, Epoch: 100},
	}
	lines := graph.Render(graph.Layout(nodes), graph.Style{})
	assert.Equal(t, indexOf(lines, "child of head"), len(lines)-1) // head-side tip -> bottom
	assert.That(t, indexOf(lines, "h head") < indexOf(lines, "child of head"), "child stays below HEAD")
	assert.That(t, indexOf(lines, "other branch") < indexOf(lines, "h head"), "unrelated newer branch floats above HEAD lineage")
}

func TestHeadFloat_NoHeadUnchanged(t *testing.T) {
	t.Parallel()
	// No IsHead set: ordering is the plain newest-at-bottom default.
	nodes := []graph.Node{
		{ID: "base", Label: "base", Parents: nil, Epoch: 1},
		{ID: "x", Label: "x older", Parents: []string{"base"}, Epoch: 5},
		{ID: "y", Label: "y newer", Parents: []string{"base"}, Epoch: 10},
	}
	lines := graph.Render(graph.Layout(nodes), graph.Style{})
	assert.Equal(t, indexOf(lines, "y newer"), len(lines)-1) // newest at bottom
}
