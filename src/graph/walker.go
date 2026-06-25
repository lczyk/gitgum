package graph

import (
	"os"
	"sort"
)

// useWalker selects the active-lanes row-walker over the legacy multi-phase
// engine. Opt-in until proven; see Layout.
var useWalker = os.Getenv("GG_GRAPH_WALKER") != ""

// ------ active-lanes row-walker ------------------------------------------------
//
// Model: walk commits newest-first maintaining `lanes` -- one column per live
// edge, each carrying the id of the parent commit it heads toward. A column
// hosts exactly one edge at a time, so occupancy is exact by construction and
// a lane can never be packed onto a column another edge crosses. That is the
// whole point: it dissolves the disconnection class the legacy compaction hit.
//
// Per commit we emit: optional fan-in connector rows (children converging),
// the commit row, optional fan-out connector rows (extra parents diverging).
// Diagonals step one column per row so every edge is a continuous path.
//
// Output is built newest-first then reversed (and slashes swapped) to match
// the oldest-first LayoutResult contract the rest of the package expects.

type laneEdge struct {
	target string // parent commit id this lane heads toward
}

type walkState struct {
	idx     map[string]*nodeState
	lanes   []*laneEdge // nil = free column
	rows    [][]Glyph   // newest-first; reversed at the end
	gaps    [][]Glyph   // parallel to rows: trailing-slot diagonals (crossings)
	commits []*Node     // parallel to rows; nil on connector rows
	width   int
}

func layoutWalker(nodes []Node) LayoutResult {
	if len(nodes) == 0 {
		return LayoutResult{}
	}
	// Reuse the legacy state construction + topo sort purely for ordering:
	// it gives each node a stable row (newest = highest) with the nice
	// "second parent right after its merge" placement.
	st := newLayoutState(nodes)
	st.sort()
	order := make([]*nodeState, len(st.nodes))
	copy(order, st.nodes)
	sort.Slice(order, func(i, j int) bool { return order[i].row > order[j].row }) // newest first

	w := &walkState{idx: st.idx}
	for _, ns := range order {
		w.place(ns)
	}

	return w.finish()
}

// place handles one commit: fan-in, commit row, fan-out.
func (w *walkState) place(ns *nodeState) {
	id := ns.ID

	// columns whose edge targets this commit (its children's edges)
	hits := w.hits(id)
	var myCol int
	if len(hits) > 0 {
		myCol = hits[0]
		// fan-in: bring the extra child lanes into myCol before the commit row
		w.collapse(myCol, hits[1:])
	} else {
		myCol = w.allocNear(0)
		w.lanes[myCol] = &laneEdge{}
	}

	// commit row
	row := w.pipeRow()
	row[myCol] = GlyphStar
	w.emit(row, nil, ns.Node)

	// parents: first reuses myCol, extras open new lanes and fan out
	parents := ns.Parents
	if len(parents) == 0 {
		w.lanes[myCol] = nil
		return
	}
	w.lanes[myCol].target = parents[0]
	newCols := make([]int, 0, len(parents)-1)
	for k := 1; k < len(parents); k++ {
		pid := parents[k]
		// If this parent already has a live lane (another child of it has been
		// placed), route the merge edge straight to that lane instead of opening
		// a fresh far column that would only have to converge back -- avoids the
		// detour overshoot.
		if tc := w.targetCol(pid); tc >= 0 {
			w.routeMerge(myCol, tc)
			continue
		}
		nc := w.allocNear(myCol + 1)
		w.lanes[nc] = &laneEdge{target: pid}
		newCols = append(newCols, nc)
	}
	if len(newCols) > 0 {
		w.fanOut(myCol, newCols)
	}
}

// targetCol returns the leftmost column whose lane already heads toward id, or
// -1 if none.
func (w *walkState) targetCol(id string) int {
	for i, e := range w.lanes {
		if e != nil && e.target == id {
			return i
		}
	}
	return -1
}

// routeMerge draws a diagonal staircase from a merge commit at `from` to an
// already-live parent lane at `to`, one column-step per row, crossing any lanes
// in between. The destination lane is left in place; the edge just joins it.
func (w *walkState) routeMerge(from, to int) {
	if to == from {
		return
	}
	dir := -1
	glyph := GlyphSlash // newest-first: moving left while descending
	if to > from {
		dir = 1
		glyph = GlyphBackslash
	}
	for p := from + dir; ; p += dir {
		row := w.pipeRow()
		gaps := make([]Glyph, len(w.lanes))
		// Weave in the gap so crossed lanes keep their pipes. Leftward steps
		// sit in the gap left of p (gap[p]); rightward in the gap left of p too
		// (gap[p-1]) -- i.e. always the gap on the side the edge entered from.
		gi := p
		if dir > 0 {
			gi = p - 1
		}
		gaps[gi] = glyph
		w.emit(row, gaps, nil)
		if p == to {
			return
		}
	}
}

// hits returns the columns whose lane targets id, left to right.
func (w *walkState) hits(id string) []int {
	var out []int
	for i, e := range w.lanes {
		if e != nil && e.target == id {
			out = append(out, i)
		}
	}
	return out
}

// allocNear returns a free column at or after `pref` (preferring pref to keep
// fan-out diagonals short), else the leftmost free column, else a new one.
func (w *walkState) allocNear(pref int) int {
	for c := pref; c < len(w.lanes); c++ {
		if w.lanes[c] == nil {
			return c
		}
	}
	for c := 0; c < len(w.lanes); c++ {
		if w.lanes[c] == nil {
			return c
		}
	}
	w.lanes = append(w.lanes, nil)
	return len(w.lanes) - 1
}

// pipeRow renders the current lane state as a row of pipes.
func (w *walkState) pipeRow() []Glyph {
	g := make([]Glyph, len(w.lanes))
	for i, e := range w.lanes {
		if e != nil {
			g[i] = GlyphPipe
		}
	}
	return g
}

// collapse converges the extra child lanes into myCol as a single diagonal that
// sweeps right-to-left one HALF-column per row (git's classic fan): the diagonal
// occupies a column's primary slot on even half-steps and the inter-column gap on
// odd ones, so lanes settle into clean verticals behind it. Each extra is merged
// (removed) as the diagonal reaches its column; lanes the diagonal has not yet
// reached stay as pipes. Survivors that aren't extras keep their pipes and are
// crossed for the one row the diagonal sits on their column.
//
// Half-column geometry: column c's pipe is at char position 2c; the gap to its
// right is 2c+1. The diagonal sweeps d from 2*maxExtra-1 (gap left of the
// furthest extra) down to 2*myCol+1 (gap right of the sink).
func (w *walkState) collapse(myCol int, extras []int) {
	if len(extras) == 0 {
		return
	}
	maxE := myCol
	for _, c := range extras {
		if c > maxE {
			maxE = c
		}
	}
	for d := 2*maxE - 1; d >= 2*myCol+1; d-- {
		// Merge any extra the diagonal has now reached (its pipe at/right of d).
		for _, c := range extras {
			if 2*c >= d && w.lanes[c] != nil {
				w.lanes[c] = nil
			}
		}
		row := w.pipeRow()
		var gaps []Glyph
		if d%2 == 1 {
			gaps = make([]Glyph, len(w.lanes))
			gaps[(d-1)/2] = GlyphSlash // diagonal in the inter-column gap
		} else {
			row[d/2] = GlyphSlash // diagonal on a column's primary slot
		}
		w.emit(row, gaps, nil)
	}
}

// fanOut draws the diagonals from a merge at myCol to its extra-parent lanes --
// the mirror of collapse. The diagonals weave in the inter-column gaps (so a
// 2-parent merge is a clean `|\`, not `| \`), sweeping one column per row toward
// each new lane. A new lane is "in transit" until the diagonal reaches it, so it
// isn't drawn as a pipe on the rows above its column.
func (w *walkState) fanOut(myCol int, newCols []int) {
	maxC := myCol
	saved := make([]*laneEdge, len(newCols))
	for i, c := range newCols {
		if c > maxC {
			maxC = c
		}
		saved[i] = w.lanes[c]
		w.lanes[c] = nil // born via the diagonal; verticalises once reached
	}
	for p := myCol + 1; p <= maxC; p++ {
		row := w.pipeRow()
		gaps := make([]Glyph, len(w.lanes))
		for i, c := range newCols {
			if p > c {
				continue // this lane already reached its column
			}
			gaps[p-1] = GlyphBackslash // diagonal in the gap left of column p
			if p == c {
				w.lanes[c] = saved[i] // settle: pipe from the next row on
			}
		}
		w.emit(row, gaps, nil)
	}
	// Any lane not yet restored (shouldn't happen: p reaches maxC >= every c).
	for i, c := range newCols {
		if w.lanes[c] == nil {
			w.lanes[c] = saved[i]
		}
	}
}

// emit appends a row (padding to the running width). gaps may be nil. Connector
// rows (no commit) that carry no diagonal -- in either the column glyphs or the
// gaps -- are pure pipes between two commit rows, redundant, so they're dropped.
func (w *walkState) emit(row []Glyph, gaps []Glyph, commit *Node) {
	if commit == nil {
		hasDiag := false
		for _, g := range row {
			if g == GlyphSlash || g == GlyphBackslash {
				hasDiag = true
				break
			}
		}
		for _, g := range gaps {
			if g == GlyphSlash || g == GlyphBackslash {
				hasDiag = true
				break
			}
		}
		if !hasDiag {
			return
		}
	}
	if len(row) > w.width {
		w.width = len(row)
	}
	w.rows = append(w.rows, row)
	w.gaps = append(w.gaps, gaps)
	w.commits = append(w.commits, commit)
}

// finish reverses to oldest-first, swaps slashes (in both column glyphs and
// gaps, since the y-flip turns every `/` into `\` and vice versa), and pads to
// width.
func (w *walkState) finish() LayoutResult {
	n := len(w.rows)
	out := make([]Row, n)
	swap := func(dst, src []Glyph) {
		for c := range dst {
			if c >= len(src) {
				continue
			}
			switch src[c] {
			case GlyphSlash:
				dst[c] = GlyphBackslash
			case GlyphBackslash:
				dst[c] = GlyphSlash
			default:
				dst[c] = src[c]
			}
		}
	}
	for i := 0; i < n; i++ {
		g := make([]Glyph, w.width)
		swap(g, w.rows[n-1-i])
		var gap []Glyph
		if src := w.gaps[n-1-i]; src != nil {
			gap = make([]Glyph, w.width)
			swap(gap, src)
		}
		out[i] = Row{Commit: w.commits[n-1-i], Glyphs: g, Gap: gap}
	}
	return LayoutResult{Rows: out, Columns: w.width}
}
