package graph_test

import (
	"fmt"
	"testing"

	"github.com/lczyk/gitgum/src/graph"
)

// linearChain builds a chain of n commits, each parent of the next.
func linearChain(n int) []graph.Node {
	nodes := make([]graph.Node, n)
	for i := 0; i < n; i++ {
		var parents []string
		if i > 0 {
			parents = []string{fmt.Sprintf("c%d", i-1)}
		}
		nodes[i] = graph.Node{
			ID:      fmt.Sprintf("c%d", i),
			Label:   fmt.Sprintf("c%d", i),
			Date:    fmt.Sprintf("2020-01-01T00:00:%02dZ", i%60),
			Parents: parents,
		}
	}
	return nodes
}

// merges builds a mainline of n commits with a side-branch + merge every
// `every` commits. Models a typical PR-merge history.
func mergeSeries(n, every int) []graph.Node {
	nodes := make([]graph.Node, 0, n*2)
	prevMain := ""
	for i := 0; i < n; i++ {
		mainID := fmt.Sprintf("m%d", i)
		var p []string
		if prevMain != "" {
			p = []string{prevMain}
		}
		nodes = append(nodes, graph.Node{
			ID: mainID, Label: mainID,
			Date: fmt.Sprintf("2020-01-01T%02d:00:00Z", i%24), Parents: p,
		})
		if i > 0 && i%every == 0 {
			sideID := fmt.Sprintf("s%d", i)
			nodes = append(nodes, graph.Node{
				ID: sideID, Label: sideID,
				Date: fmt.Sprintf("2020-01-01T%02d:30:00Z", i%24), Parents: []string{prevMain},
			})
			mergeID := fmt.Sprintf("M%d", i)
			nodes = append(nodes, graph.Node{
				ID: mergeID, Label: mergeID,
				Date: fmt.Sprintf("2020-01-01T%02d:45:00Z", i%24), Parents: []string{mainID, sideID},
			})
			prevMain = mergeID
		} else {
			prevMain = mainID
		}
	}
	return nodes
}

// parallelBranches builds k branches of length len each, all forking from a
// shared root. Stresses col allocation + compaction.
func parallelBranches(k, length int) []graph.Node {
	nodes := []graph.Node{{ID: "root", Label: "root", Date: "2020-01-01T00:00:00Z"}}
	for b := 0; b < k; b++ {
		prev := "root"
		for i := 0; i < length; i++ {
			id := fmt.Sprintf("b%dc%d", b, i)
			nodes = append(nodes, graph.Node{
				ID: id, Label: id,
				Date:    fmt.Sprintf("2020-01-%02dT00:00:%02dZ", b+1, i%60),
				Parents: []string{prev},
			})
			prev = id
		}
	}
	return nodes
}

func benchLayout(b *testing.B, nodes []graph.Node) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = graph.Layout(nodes)
	}
}

func benchLayoutAndRender(b *testing.B, nodes []graph.Node) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lr := graph.Layout(nodes)
		_ = graph.Render(lr, nil)
	}
}

func BenchmarkLayout_Linear100(b *testing.B)   { benchLayout(b, linearChain(100)) }
func BenchmarkLayout_Linear1000(b *testing.B)  { benchLayout(b, linearChain(1000)) }
func BenchmarkLayout_Linear10000(b *testing.B) { benchLayout(b, linearChain(10000)) }

func BenchmarkLayout_Merges100(b *testing.B)  { benchLayout(b, mergeSeries(100, 5)) }
func BenchmarkLayout_Merges1000(b *testing.B) { benchLayout(b, mergeSeries(1000, 5)) }

func BenchmarkLayout_Parallel10x10(b *testing.B)  { benchLayout(b, parallelBranches(10, 10)) }
func BenchmarkLayout_Parallel100x10(b *testing.B) { benchLayout(b, parallelBranches(100, 10)) }
func BenchmarkLayout_Parallel10x100(b *testing.B) { benchLayout(b, parallelBranches(10, 100)) }

func BenchmarkRender_Linear1000(b *testing.B)  { benchLayoutAndRender(b, linearChain(1000)) }
func BenchmarkRender_Merges1000(b *testing.B)  { benchLayoutAndRender(b, mergeSeries(1000, 5)) }
func BenchmarkRender_Parallel100x10(b *testing.B) {
	benchLayoutAndRender(b, parallelBranches(100, 10))
}

// benchRenderOnly measures Render alone -- Layout is run once outside the
// loop so regressions in the per-row formatter / slot packer surface
// independently of layout work.
func benchRenderOnly(b *testing.B, nodes []graph.Node, cs graph.ColorScheme) {
	lr := graph.Layout(nodes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = graph.Render(lr, cs)
	}
}

func BenchmarkRenderOnly_Linear1000(b *testing.B)  { benchRenderOnly(b, linearChain(1000), nil) }
func BenchmarkRenderOnly_Merges1000(b *testing.B)  { benchRenderOnly(b, mergeSeries(1000, 5), nil) }
func BenchmarkRenderOnly_Parallel100x10(b *testing.B) {
	benchRenderOnly(b, parallelBranches(100, 10), nil)
}

// idColorScheme wraps each fragment in a kind-prefixed marker. Cheap but
// non-trivial -- exercises the call-per-fragment path real callers hit.
func idColorScheme(kind graph.GlyphKind, text string) string {
	switch kind {
	case graph.KindGraph:
		return "<g>" + text + "</>"
	case graph.KindHash:
		return "<h>" + text + "</>"
	case graph.KindRef:
		return "<r>" + text + "</>"
	case graph.KindSubject:
		return "<s>" + text + "</>"
	}
	return text
}

func BenchmarkRenderOnly_Linear1000_Color(b *testing.B) {
	benchRenderOnly(b, linearChain(1000), idColorScheme)
}
func BenchmarkRenderOnly_Merges1000_Color(b *testing.B) {
	benchRenderOnly(b, mergeSeries(1000, 5), idColorScheme)
}
