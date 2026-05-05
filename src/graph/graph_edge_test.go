package graph_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/lczyk/assert"
	"github.com/lczyk/gitgum/src/graph"
)

// graph_edge_test.go pins behavior on pathological inputs (cycles,
// self-parents, missing parents, duplicate IDs) and on contract
// properties (determinism, concurrency).

// TestEdge_CyclicGraph: A->B->A cycle. Engine should not loop forever.
// Output is unspecified, but Layout must terminate and Render must
// produce as many commit rows as input nodes.
func TestEdge_CyclicGraph(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "a", Label: "a", Parents: []string{"b"}, Epoch: 1},
		{ID: "b", Label: "b", Parents: []string{"a"}, Epoch: 2},
	}
	lr := graph.Layout(nodes)
	commitRows := 0
	for _, r := range lr.Rows {
		if r.Commit != nil {
			commitRows++
		}
	}
	assert.Equal(t, commitRows, 2)
}

// TestEdge_SelfParent: a node listing itself as parent. Engine must not
// loop / panic.
func TestEdge_SelfParent(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "a", Label: "a", Parents: []string{"a"}, Epoch: 1},
	}
	lr := graph.Layout(nodes)
	assert.That(t, len(lr.Rows) >= 1, "at least one row")
}

// TestEdge_MissingParent: child references a parent ID not in the input.
// Engine should silently skip the dangling edge and render the child as
// if it were a root.
func TestEdge_MissingParent(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "child", Label: "child", Parents: []string{"phantom"}, Epoch: 1},
	}
	lr := graph.Layout(nodes)
	lines := graph.Render(lr, graph.Style{})
	assert.Equal(t, len(lines), 1)
	assert.That(t, strings.Contains(lines[0], "child"), "child line present")
}

// TestEdge_DuplicateID: two nodes share an ID. Engine panics with a
// clear message rather than silently producing garbage.
func TestEdge_DuplicateID(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "a", Label: "a-first", Epoch: 1},
		{ID: "a", Label: "a-second", Epoch: 2},
	}
	assert.Panic(t, func() { graph.Layout(nodes) }, func(t testing.TB, rec any) {
		assert.That(t, strings.Contains(rec.(string), "duplicate"), "panic should mention duplicate, got %q", rec)
	})
}

// TestEdge_Determinism: Layout(nodes) twice must produce byte-identical
// rendered output. Catches non-determinism creeping in via map iteration.
func TestEdge_Determinism(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "base", Label: "base", Epoch: 1},
		{ID: "a1", Label: "a1", Epoch: 2, Parents: []string{"base"}, Lane: h("a")},
		{ID: "b1", Label: "b1", Epoch: 3, Parents: []string{"base"}, Lane: h("b")},
		{ID: "merge", Label: "merge", Epoch: 4, Parents: []string{"a1", "b1"}, Lane: h("a")},
	}
	first := strings.Join(graph.Render(graph.Layout(nodes), graph.Style{}), "\n")
	for i := 0; i < 50; i++ {
		out := strings.Join(graph.Render(graph.Layout(nodes), graph.Style{}), "\n")
		if out != first {
			t.Fatalf("nondeterministic output on iteration %d:\n--- first ---\n%s\n--- got ---\n%s", i, first, out)
		}
	}
}

// TestEdge_NoEpoch: Layout works without per-node sort keys. With every
// Epoch at zero the engine tiebreaks by ID, producing deterministic
// output without falling over.
func TestEdge_NoEpoch(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "base", Label: "base"},
		{ID: "a", Label: "a", Parents: []string{"base"}},
		{ID: "b", Label: "b", Parents: []string{"base"}},
		{ID: "merge", Label: "merge", Parents: []string{"a", "b"}},
	}
	first := strings.Join(graph.Render(graph.Layout(nodes), graph.Style{}), "\n")
	assert.That(t, strings.Contains(first, "base"), "base in output")
	assert.That(t, strings.Contains(first, "merge"), "merge in output")
	for i := 0; i < 20; i++ {
		out := strings.Join(graph.Render(graph.Layout(nodes), graph.Style{}), "\n")
		if out != first {
			t.Fatalf("nondeterministic output without Epoch on iteration %d:\n%s", i, out)
		}
	}
}

// TestEdge_ConcurrentLayout: Layout has no shared state across calls and
// must be safe to run from many goroutines on the same input.
func TestEdge_ConcurrentLayout(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "base", Label: "base", Epoch: 1},
		{ID: "main1", Label: "main1", Epoch: 2, Parents: []string{"base"}},
		{ID: "side1", Label: "side1", Epoch: 3, Parents: []string{"base"}},
		{ID: "merge", Label: "merge", Epoch: 4, Parents: []string{"main1", "side1"}},
	}
	want := strings.Join(graph.Render(graph.Layout(nodes), graph.Style{}), "\n")
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := strings.Join(graph.Render(graph.Layout(nodes), graph.Style{}), "\n")
			if got != want {
				t.Errorf("concurrent layout produced different output:\n%s", got)
			}
		}()
	}
	wg.Wait()
}
