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
			Epoch:   int64(i),
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
			Epoch: int64(i * 100), Parents: p,
		})
		if i > 0 && i%every == 0 {
			sideID := fmt.Sprintf("s%d", i)
			nodes = append(nodes, graph.Node{
				ID: sideID, Label: sideID,
				Epoch: int64(i*100 + 30), Parents: []string{prevMain},
			})
			mergeID := fmt.Sprintf("M%d", i)
			nodes = append(nodes, graph.Node{
				ID: mergeID, Label: mergeID,
				Epoch: int64(i*100 + 45), Parents: []string{mainID, sideID},
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
	nodes := []graph.Node{{ID: "root", Label: "root", Epoch: 0}}
	for b := 0; b < k; b++ {
		prev := "root"
		for i := 0; i < length; i++ {
			id := fmt.Sprintf("b%dc%d", b, i)
			nodes = append(nodes, graph.Node{
				ID: id, Label: id,
				Epoch:   int64((b+1)*1000 + i),
				Parents: []string{prev},
			})
			prev = id
		}
	}
	return nodes
}

// backAndForthCatchups builds n back-and-forth catch-up cycles between a
// main lane and a feat lane. Each cycle: main commit, then feat catches
// up to main, then main catches up to feat. Stresses crossing-col reuse:
// each catch-up needs a routing col, but they're non-overlapping in time
// so a single routing col should serve all of them.
func backAndForthCatchups(n int) []graph.Node {
	nodes := []graph.Node{{ID: "A", Label: "A", Epoch: 0, Lane: 1}}
	mainPrev := "A"
	featPrev := ""
	for i := 0; i < n; i++ {
		f1 := fmt.Sprintf("f%d", i)
		nodes = append(nodes, graph.Node{
			ID: f1, Label: f1, Epoch: int64(i*4 + 1),
			Parents: []string{
				func() string {
					if featPrev == "" {
						return mainPrev
					}
					return featPrev
				}(),
			},
			Lane: 2,
		})
		c := fmt.Sprintf("c%d", i)
		nodes = append(nodes, graph.Node{
			ID: c, Label: c, Epoch: int64(i*4 + 2),
			Parents: []string{mainPrev, f1}, Lane: 1,
		})
		mainPrev = c
		featPrev = f1
	}
	return nodes
}

// sequentialSideBranches builds n short PR-merge style side branches off
// a shared base. Each side branch is a single commit merged via a 2-parent
// merge back to main. Stresses pushed-intro-to-commit-row behavior and
// lane reuse in col 1.
func sequentialSideBranches(n int) []graph.Node {
	nodes := []graph.Node{{ID: "base", Label: "base", Epoch: 0, Lane: 1}}
	prev := "base"
	for i := 0; i < n; i++ {
		s := fmt.Sprintf("s%d", i)
		m := fmt.Sprintf("M%d", i)
		nodes = append(nodes, graph.Node{
			ID: s, Label: s, Epoch: int64(i*2 + 1),
			Parents: []string{prev}, Lane: 2,
		})
		nodes = append(nodes, graph.Node{
			ID: m, Label: m, Epoch: int64(i*2 + 2),
			Parents: []string{prev, s}, Lane: 1,
		})
		prev = m
	}
	return nodes
}

// sharedParentDualMerge mirrors the rocks-security-manifest topology:
// an outer merge whose first parent (m_tip) is also the second parent of
// an inner merge nested in the outer's other-parent subtree. Repeats the
// pattern n times to stress the topo-sort pre-decrement path.
func sharedParentDualMerge(n int) []graph.Node {
	nodes := []graph.Node{{ID: "root", Label: "root", Epoch: 0, Lane: 1}}
	prev := "root"
	for i := 0; i < n; i++ {
		mTip := fmt.Sprintf("m%d", i)
		fTip := fmt.Sprintf("f%d", i)
		inner := fmt.Sprintf("inner%d", i)
		outer := fmt.Sprintf("outer%d", i)
		nodes = append(nodes,
			graph.Node{ID: mTip, Label: mTip, Epoch: int64(i*5 + 1), Parents: []string{prev}, Lane: 1},
			graph.Node{ID: fTip, Label: fTip, Epoch: int64(i*5 + 2), Parents: []string{prev}, Lane: 2},
			graph.Node{ID: inner, Label: inner, Epoch: int64(i*5 + 3), Parents: []string{fTip, mTip}, Lane: 2},
			graph.Node{ID: outer, Label: outer, Epoch: int64(i*5 + 4), Parents: []string{mTip, inner}, Lane: 1},
		)
		prev = outer
	}
	return nodes
}

// octopusFan builds a single k-parent octopus merge. All k-1 non-first
// parents are single-commit branches off a shared root; after compaction
// they share col 1, exercising the term-stagger dedup path.
func octopusFan(k int) []graph.Node {
	nodes := []graph.Node{{ID: "A", Label: "A", Epoch: 0, Lane: 1}}
	parents := []string{"A"}
	for i := 1; i < k; i++ {
		id := fmt.Sprintf("p%d", i)
		nodes = append(nodes, graph.Node{
			ID: id, Label: id, Epoch: int64(i),
			Parents: []string{"A"}, Lane: int64(i + 2),
		})
		parents = append(parents, id)
	}
	nodes = append(nodes, graph.Node{
		ID: "M", Label: "M", Epoch: int64(k + 1),
		Parents: parents, Lane: 1,
	})
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
		_ = graph.Render(lr, graph.Style{})
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

func BenchmarkRender_Linear1000(b *testing.B) { benchLayoutAndRender(b, linearChain(1000)) }
func BenchmarkRender_Merges1000(b *testing.B) { benchLayoutAndRender(b, mergeSeries(1000, 5)) }
func BenchmarkRender_Parallel100x10(b *testing.B) {
	benchLayoutAndRender(b, parallelBranches(100, 10))
}

// benchRenderOnly measures Render alone -- Layout is run once outside the
// loop so regressions in the per-row formatter / slot packer surface
// independently of layout work.
func benchRenderOnly(b *testing.B, nodes []graph.Node, st graph.Style) {
	lr := graph.Layout(nodes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = graph.Render(lr, st)
	}
}

func BenchmarkRenderOnly_Linear1000(b *testing.B) {
	benchRenderOnly(b, linearChain(1000), graph.Style{})
}
func BenchmarkRenderOnly_Merges1000(b *testing.B) {
	benchRenderOnly(b, mergeSeries(1000, 5), graph.Style{})
}
func BenchmarkRenderOnly_Parallel100x10(b *testing.B) {
	benchRenderOnly(b, parallelBranches(100, 10), graph.Style{})
}

// styled wraps line/star glyphs. Cheap markers exercise the styled
// branch in writeSlots without simulating a full ANSI scheme.
var styled = graph.Style{
	LinePrefix: "<L>", LineSuffix: "</L>",
	StarPrefix: "<S>", StarSuffix: "</S>",
}

func BenchmarkRenderOnly_Linear1000_Color(b *testing.B) {
	benchRenderOnly(b, linearChain(1000), styled)
}
func BenchmarkRenderOnly_Merges1000_Color(b *testing.B) {
	benchRenderOnly(b, mergeSeries(1000, 5), styled)
}

// Benches for topology classes that previously triggered bugs --
// shared-parent dual-merge (topo ordering), back-and-forth catch-ups
// (routing-col reuse), sequential side branches (pushed-intro-to-commit
// behavior), octopus fans (term-stagger dedup).
func BenchmarkLayout_SharedDual100(b *testing.B)  { benchLayout(b, sharedParentDualMerge(100)) }
func BenchmarkLayout_SharedDual1000(b *testing.B) { benchLayout(b, sharedParentDualMerge(1000)) }

func BenchmarkLayout_BackForth100(b *testing.B)  { benchLayout(b, backAndForthCatchups(100)) }
func BenchmarkLayout_BackForth1000(b *testing.B) { benchLayout(b, backAndForthCatchups(1000)) }

func BenchmarkLayout_SeqSides100(b *testing.B)  { benchLayout(b, sequentialSideBranches(100)) }
func BenchmarkLayout_SeqSides1000(b *testing.B) { benchLayout(b, sequentialSideBranches(1000)) }

func BenchmarkLayout_Octopus10(b *testing.B)  { benchLayout(b, octopusFan(10)) }
func BenchmarkLayout_Octopus100(b *testing.B) { benchLayout(b, octopusFan(100)) }

func BenchmarkRender_BackForth1000(b *testing.B) {
	benchLayoutAndRender(b, backAndForthCatchups(1000))
}
func BenchmarkRender_SeqSides1000(b *testing.B) {
	benchLayoutAndRender(b, sequentialSideBranches(1000))
}
func BenchmarkRender_SharedDual1000(b *testing.B) {
	benchLayoutAndRender(b, sharedParentDualMerge(1000))
}
func BenchmarkRender_Octopus100(b *testing.B) { benchLayoutAndRender(b, octopusFan(100)) }
