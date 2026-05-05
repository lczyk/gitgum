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
		{ID: "a", Label: "a", Parents: []string{"b"}, Date: "2020-01-01T00:00:01Z"},
		{ID: "b", Label: "b", Parents: []string{"a"}, Date: "2020-01-01T00:00:02Z"},
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
		{ID: "a", Label: "a", Parents: []string{"a"}, Date: "2020-01-01T00:00:01Z"},
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
		{ID: "child", Label: "child", Parents: []string{"phantom"}, Date: "2020-01-01T00:00:01Z"},
	}
	lr := graph.Layout(nodes)
	lines := graph.Render(lr, nil)
	assert.Equal(t, len(lines), 1)
	assert.That(t, strings.Contains(lines[0], "child"), "child line present")
}

// TestEdge_DuplicateID: two nodes share an ID. Engine panics with a
// clear message rather than silently producing garbage.
func TestEdge_DuplicateID(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "a", Label: "a-first", Date: "2020-01-01T00:00:01Z"},
		{ID: "a", Label: "a-second", Date: "2020-01-01T00:00:02Z"},
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate ID, got none")
		}
		if !strings.Contains(r.(string), "duplicate") {
			t.Errorf("panic message should mention duplicate, got %q", r)
		}
	}()
	graph.Layout(nodes)
}

// TestEdge_Determinism: Layout(nodes) twice must produce byte-identical
// rendered output. Catches non-determinism creeping in via map iteration.
func TestEdge_Determinism(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "base", Label: "base", Date: "2020-01-01T00:00:01Z"},
		{ID: "a1", Label: "a1", Date: "2020-01-01T00:00:02Z", Parents: []string{"base"}, LayoutHint: "a"},
		{ID: "b1", Label: "b1", Date: "2020-01-01T00:00:03Z", Parents: []string{"base"}, LayoutHint: "b"},
		{ID: "merge", Label: "merge", Date: "2020-01-01T00:00:04Z", Parents: []string{"a1", "b1"}, LayoutHint: "a"},
	}
	first := strings.Join(graph.Render(graph.Layout(nodes), nil), "\n")
	for i := 0; i < 50; i++ {
		out := strings.Join(graph.Render(graph.Layout(nodes), nil), "\n")
		if out != first {
			t.Fatalf("nondeterministic output on iteration %d:\n--- first ---\n%s\n--- got ---\n%s", i, first, out)
		}
	}
}

// TestEdge_ConcurrentLayout: Layout has no shared state across calls and
// must be safe to run from many goroutines on the same input.
func TestEdge_ConcurrentLayout(t *testing.T) {
	t.Parallel()
	nodes := []graph.Node{
		{ID: "base", Label: "base", Date: "2020-01-01T00:00:01Z"},
		{ID: "main1", Label: "main1", Date: "2020-01-01T00:00:02Z", Parents: []string{"base"}},
		{ID: "side1", Label: "side1", Date: "2020-01-01T00:00:03Z", Parents: []string{"base"}},
		{ID: "merge", Label: "merge", Date: "2020-01-01T00:00:04Z", Parents: []string{"main1", "side1"}},
	}
	want := strings.Join(graph.Render(graph.Layout(nodes), nil), "\n")
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := strings.Join(graph.Render(graph.Layout(nodes), nil), "\n")
			if got != want {
				t.Errorf("concurrent layout produced different output:\n%s", got)
			}
		}()
	}
	wg.Wait()
}
