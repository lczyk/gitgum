package graph_test

import (
	"fmt"

	"github.com/lczyk/gitgum/src/graph"
)

// A linear chain of three nodes renders as a single column, oldest at the top.
func ExampleLayout() {
	nodes := []graph.Node{
		{ID: "a", Label: "a first", Epoch: 1},
		{ID: "b", Label: "b second", Parents: []string{"a"}, Epoch: 2},
		{ID: "c", Label: "c third", Parents: []string{"b"}, Epoch: 3},
	}
	for _, line := range graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{}) {
		fmt.Println(line)
	}
	// Output:
	// * a first
	// * b second
	// * c third
}

// A fork and a merge: `side` branches off `base`, `merge` joins it back to
// `main`. Parents[0] is the primary edge, so the merge stays in `main`'s lane
// and the secondary parent fans out to its own column.
func ExampleLayout_merge() {
	nodes := []graph.Node{
		{ID: "base", Label: "base", Epoch: 1},
		{ID: "side", Label: "side work", Parents: []string{"base"}, Epoch: 2},
		{ID: "main", Label: "main work", Parents: []string{"base"}, Epoch: 3},
		{ID: "merge", Label: "merge side", Parents: []string{"main", "side"}, Epoch: 4},
	}
	for _, line := range graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{}) {
		fmt.Println(line)
	}
	// Output:
	// * base
	// v\
	// * | main work
	// | * side work
	// v/
	// * merge side
}

// Style wraps the graph glyphs in ANSI escapes; Label is emitted verbatim, so
// callers wanting per-segment colouring embed the escapes in Label themselves.
// Angle-bracket markers stand in for the escapes here.
func ExampleRender_style() {
	nodes := []graph.Node{
		{ID: "base", Label: "base", Epoch: 1},
		{ID: "side", Label: "side work", Parents: []string{"base"}, Epoch: 2},
		{ID: "main", Label: "main work", Parents: []string{"base"}, Epoch: 3},
		{ID: "merge", Label: "merge side", Parents: []string{"main", "side"}, Epoch: 4},
	}
	st := graph.Style{LinePrefix: "<line>", LineSuffix: "</line>", StarPrefix: "<dot>", StarSuffix: "</dot>"}
	for _, line := range graph.Render(graph.Layout(nodes, graph.Opt{}), st) {
		fmt.Println(line)
	}
	// Output:
	// <dot>*</dot> base
	// <dot>v</dot><line>\</line>
	// <dot>*</dot> <line>|</line> main work
	// <line>|</line> <dot>*</dot> side work
	// <dot>v</dot><line>/</line>
	// <dot>*</dot> merge side
}

// Node.Float sinks a node and its descendants to the bottom of the output,
// even when a newer node would otherwise land there.
func ExampleNode_float() {
	nodes := []graph.Node{
		{ID: "base", Label: "base", Epoch: 1},
		{ID: "x", Label: "x floated", Parents: []string{"base"}, Epoch: 5, Float: true},
		{ID: "y", Label: "y newer", Parents: []string{"base"}, Epoch: 10},
	}
	for _, line := range graph.Render(graph.Layout(nodes, graph.Opt{}), graph.Style{}) {
		fmt.Println(line)
	}
	// Output:
	// * base
	// v\
	// | * y newer
	// * x floated
}
