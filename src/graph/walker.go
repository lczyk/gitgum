package graph

import (
	"slices"
	"sort"
)

// ------ active-lanes row-walker ------------------------------------------------
//
// Model: walk nodes newest-first maintaining `lanes` -- one column per live
// edge, each carrying the id of the parent node it heads toward. A column
// hosts exactly one edge at a time, so occupancy is exact by construction and
// a lane can never be packed onto a column another edge crosses. That is the
// whole point: it dissolves the disconnection class the legacy compaction hit.
//
// Per node we emit: optional fan-in connector rows (children converging),
// the node row, optional fan-out connector rows (extra parents diverging).
// Diagonals step one column per row so every edge is a continuous path.
//
// Output is built newest-first then reversed (and slashes swapped) to match
// the oldest-first LayoutResult contract the rest of the package expects.
//
// Column choice follows that newest-first walk, so in the default oldest-first
// output a lane can be born well to the right of columns that look free beside
// it: those columns were occupied when the lane was allocated, and their own
// lanes end higher up the page. The gutter is wide, not broken -- read the
// lane's own endpoints before suspecting a dropped edge.

type laneEdge struct {
	target string // parent node id this lane heads toward
}

type walkState struct {
	lanes    []*laneEdge // nil = free column
	rows     [][]Glyph   // newest-first; reversed at the end
	gaps     [][]Glyph   // parallel to rows: trailing-slot diagonals (crossings)
	rowNodes []*Node     // parallel to rows; nil on connector rows
	width    int
	opt      Opt
}

func layoutWalker(nodes []Node, opt Opt) LayoutResult {
	if len(nodes) == 0 {
		return LayoutResult{}
	}
	// The topo sort (engine.go) fixes row order only: each node gets a stable
	// row (newest = highest) with the nice "secondary parent right after its
	// merge" placement. Column assignment is all done here in the walker.
	st := newLayoutState(nodes)
	st.sort()
	order := make([]*nodeState, len(st.nodes))
	copy(order, st.nodes)
	sort.Slice(order, func(i, j int) bool { return order[i].row > order[j].row }) // newest first

	// Every node emits a row and fans add connector rows on top, so len(nodes)
	// is a floor rather than a bound -- but starting there is what keeps emit
	// off the regrowth treadmill, which dominated the walker's allocation.
	w := &walkState{
		opt:      opt,
		rows:     make([][]Glyph, 0, len(nodes)),
		gaps:     make([][]Glyph, 0, len(nodes)),
		rowNodes: make([]*Node, 0, len(nodes)),
	}
	for _, ns := range order {
		w.place(ns)
	}

	return w.finish()
}

// place handles one node: fan-in, node row, fan-out.
func (w *walkState) place(ns *nodeState) {
	id := ns.ID

	// columns whose edge targets this node (its children's edges)
	hits := w.hits(id)
	var myCol int
	if len(hits) > 0 {
		myCol = hits[0]
		// fan-in: bring the extra child lanes into myCol before the node row
		w.collapse(myCol, hits[1:])
	} else {
		myCol = w.allocNear(0)
		w.lanes[myCol] = &laneEdge{}
	}

	// node row
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

// diag is the shared half-column diagonal emitter. It sweeps a front position
// `d` (in half-column units: column c's pipe sits at 2c, the gap to its right
// at 2c+1) from dStart to dEnd inclusive, one half-step per row, so every
// diagonal in the layout climbs at the same angle -- gap slot on odd d (`|\|`
// weave), column primary on even d (a bare `\` crossing that column's slot).
//
// The even-d rows overwrite whatever sits in that column, live lane included:
// a diagonal passing over an occupied column replaces its pipe for that single
// row, so the lane reads as pipe / diagonal / pipe down the page. That is the
// intended crossover look, not a severed lane -- don't "fix" it by skipping the
// row or the diagonal loses a step and stops being a continuous path.
// beforeRow, if set, runs just before each row is built so callers can update
// lane state (merge / birth) as the front reaches a column.
//
// split lists columns whose pipe should render as a split marker on the row
// where the front sits in the gap beside them -- the row on which the diagonal
// parts from that lane, one row below the lane's undivided stretch. The front
// only ever passes one side of a split column while that column still draws a
// pipe, so plain adjacency (|d - 2c| == 1) picks the right row from either
// direction; the pipe check rejects the far side, where the lane is gone, and
// with it any lane that never drew a pipe within the sweep at all.
func (w *walkState) diag(dStart, dEnd int, glyph Glyph, beforeRow func(d int), split []int) {
	step := 1
	if dEnd < dStart {
		step = -1
	}
	for d := dStart; ; d += step {
		if beforeRow != nil {
			beforeRow(d)
		}
		row := w.pipeRow()
		var gaps []Glyph
		if d%2 == 1 {
			gaps = make([]Glyph, len(w.lanes))
			gaps[(d-1)/2] = glyph // diagonal in the inter-column gap
			for _, c := range split {
				if abs(d-2*c) == 1 && row[c] == GlyphPipe {
					row[c] = GlyphCaret
				}
			}
		} else {
			row[d/2] = glyph // diagonal crossing a column's primary slot
		}
		w.emit(row, gaps, nil)
		if d == dEnd {
			return
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// routeMerge draws a diagonal from a merge node at `from` to an already-live
// parent lane at `to`, crossing any lanes in between at the shared half-column
// slope. The edge leaves `from` and arrives beside `to` (both endpoints in the
// adjacent gap), so the lanes at `from` and `to` keep their pipes and the edge
// just joins the destination.
//
// The `from` end abuts the merge node's own `*`; the `to` end parts from a
// bare stretch of its lane, so `to` takes the split marker. Only `to` -- the
// lanes the diagonal crosses on the way are merely passed over, not forked.
func (w *walkState) routeMerge(from, to int) {
	if to == from {
		return
	}
	split := w.splitCols(nil, to)
	if to > from {
		w.diag(2*from+1, 2*to-1, GlyphBackslash, nil, split) // rightward, descending
	} else {
		w.diag(2*from-1, 2*to+1, GlyphSlash, nil, split) // leftward, descending
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

// collapse converges the extra child lanes into myCol as a single half-column
// diagonal sweeping right-to-left (git's classic fan). Each extra is merged
// (removed) as the front reaches its column; lanes the front has not yet
// reached stay as pipes, non-extra survivors are crossed for the one row the
// diagonal sits on their column. The front runs from the gap left of the
// furthest extra (2*maxE-1) down to the gap right of the sink (2*myCol+1).
//
// The sink and each extra is a lane the fan parts from, so each takes a split
// marker on its final pipe row -- `v\` for the sink, `| v\` and out for the
// extras. The furthest extra never gets one: the front starts already past its
// pipe, so its first drawn row is its own node.
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
	w.diag(2*maxE-1, 2*myCol+1, GlyphSlash, func(d int) {
		// Absorb any extra the front has now reached (its pipe at/right of d).
		for _, c := range extras {
			if 2*c >= d && w.lanes[c] != nil {
				w.lanes[c] = nil
			}
		}
	}, w.splitCols(extras, myCol))
}

// splitCols names the columns a fan marks: its fan-arm lanes plus its own, which
// forks (or receives) an edge just like they do and so is marked on the same
// terms. Copies rather than appending in place, since callers keep using cols
// after this. Every split marker in the layout is routed through here, so
// Opt.NoSplitMarks switches them all off in one place.
func (w *walkState) splitCols(cols []int, myCol int) []int {
	if w.opt.NoSplitMarks {
		return nil
	}
	out := make([]int, 0, len(cols)+1)
	out = append(out, cols...)
	return append(out, myCol)
}

// fanOut draws the diagonal from a merge at myCol out to its extra-parent lanes
// -- the mirror of collapse -- at the same half-column slope. Each new lane is
// "in transit" (nil, undrawn) until the front reaches it, then settles into a
// pipe. The front runs from the gap right of myCol (2*myCol+1) up to the gap
// left of the furthest new col (2*maxC-1).
//
// Being collapse's mirror, the marker rule mirrors too: the merge's own column
// and each new lane take one on their last pipe row before the diagonal claims
// the column below (`v/` at the sink, `| v/` and out). The furthest lane is in
// transit for the whole sweep, so it never draws a pipe to mark -- its edge is
// spoken for by the marker on the lane to its left.
func (w *walkState) fanOut(myCol int, newCols []int) {
	if len(newCols) == 0 {
		return
	}
	maxC := myCol
	saved := make([]*laneEdge, len(newCols))
	for i, c := range newCols {
		if c > maxC {
			maxC = c
		}
		saved[i] = w.lanes[c]
		w.lanes[c] = nil // born via the diagonal; verticalises once reached
	}
	w.diag(2*myCol+1, 2*maxC-1, GlyphBackslash, func(d int) {
		// Settle any new lane the front has now reached (mirror of collapse).
		for i, c := range newCols {
			if 2*c <= d && w.lanes[c] == nil {
				w.lanes[c] = saved[i] // pipe from this row on
			}
		}
	}, w.splitCols(newCols, myCol))
	// Any lane the sweep didn't reach (its col is at/beyond maxC's gap) settles now.
	for i, c := range newCols {
		if w.lanes[c] == nil {
			w.lanes[c] = saved[i]
		}
	}
}

// emit appends a row (padding to the running width). gaps may be nil. Connector
// rows (no node) that carry neither a diagonal nor a split marker -- in either
// the column glyphs or the gaps -- are pure pipes between two node rows,
// redundant, so they're dropped.
func (w *walkState) emit(row []Glyph, gaps []Glyph, node *Node) {
	if node == nil {
		drawn := func(g Glyph) bool {
			return g == GlyphSlash || g == GlyphBackslash || isSplitMark(g)
		}
		if !slices.ContainsFunc(row, drawn) && !slices.ContainsFunc(gaps, drawn) {
			return
		}
	}
	if len(row) > w.width {
		w.width = len(row)
	}
	w.rows = append(w.rows, row)
	w.gaps = append(w.gaps, gaps)
	w.rowNodes = append(w.rowNodes, node)
}

// finish turns the walker's newest-first rows into the requested display order
// and pads them to width. The default is oldest-first, which means reversing the
// rows and mirroring every glyph -- in both column glyphs and gaps, since the
// y-flip turns each `/` into `\`, each `^` into `v`, and vice versa.
//
// Opt.Reverse asks for newest-first, which is the order the walker already built
// and the orientation it already drew, so that case is simply the flip not
// happening. Reversing twice would land back here anyway.
func (w *walkState) finish() LayoutResult {
	n := len(w.rows)
	out := make([]Row, n)
	flip := !w.opt.Reverse
	copyRow := func(dst, src []Glyph) {
		for c := range dst {
			if c >= len(src) {
				continue
			}
			if flip {
				dst[c] = mirror(src[c])
			} else {
				dst[c] = src[c]
			}
		}
	}
	// Every row is the same width and none of them outlive the result, so the
	// glyphs come out of one backing array each. Slices are capped to their
	// own row: a caller appending to Glyphs or Gap gets a copy rather than the
	// next row's cells.
	glyphBuf := make([]Glyph, n*w.width)
	gapRows := 0
	for _, s := range w.gaps {
		if s != nil {
			gapRows++
		}
	}
	gapBuf := make([]Glyph, gapRows*w.width)

	for i := range n {
		src := i // newest-first, as built
		if flip {
			src = n - 1 - i
		}
		g := glyphBuf[:w.width:w.width]
		glyphBuf = glyphBuf[w.width:]
		copyRow(g, w.rows[src])
		var gap []Glyph
		if s := w.gaps[src]; s != nil {
			gap = gapBuf[:w.width:w.width]
			gapBuf = gapBuf[w.width:]
			copyRow(gap, s)
		}
		out[i] = Row{Node: w.rowNodes[src], Glyphs: g, Gap: gap}
	}
	return LayoutResult{Rows: out, Columns: w.width}
}
