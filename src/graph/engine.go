package graph

import "sort"

type Engine struct{}

func (e *Engine) Layout(g Graph) LayoutResult {
	if len(g.Nodes) == 0 {
		return LayoutResult{}
	}

	st := &layoutState{
		idx:   make(map[string]*nodeState, len(g.Nodes)),
		nodes: make([]*nodeState, 0, len(g.Nodes)),
	}

	// Count children per parent ahead of time so we can pre-size each
	// children slice exactly. Saves per-append slice-growth allocs in
	// scenarios where parents have multiple children.
	childCount := make(map[string]int, len(g.Nodes))
	for _, n := range g.Nodes {
		for _, pid := range n.Parents {
			childCount[pid]++
		}
	}

	// Slab-allocate nodeState in one shot, then hand out pointers into it.
	// Replaces N small heap allocs with one.
	slab := make([]nodeState, len(g.Nodes))
	for i := range g.Nodes {
		n := &g.Nodes[i]
		ns := &slab[i]
		ns.Node = n
		ns.row = -1
		ns.col = -1
		if c := childCount[n.ID]; c > 0 {
			ns.children = make([]*nodeState, 0, c)
		}
		st.idx[n.ID] = ns
		st.nodes = append(st.nodes, ns)
	}

	// Build child→parent reverse edges. Iterate st.nodes (deterministic
	// order) instead of st.idx (random map iteration) so children land in
	// a stable sequence; avoids needing the post-sort entirely for cases
	// where Date/ID ordering already aligns.
	for _, ns := range st.nodes {
		for _, pid := range ns.Parents {
			if p, ok := st.idx[pid]; ok {
				p.children = append(p.children, ns)
			}
		}
	}
	// Sort children deterministically: first-parent children first (mainline
	// continuity), then by date, then by ID. Map iteration above randomizes
	// append order; without this every phase that iterates children is unstable.
	// Using sort.Sort with a typed interface avoids the per-call closure
	// allocation that sort.SliceStable would incur.
	for _, ns := range st.nodes {
		if len(ns.children) < 2 {
			continue
		}
		sort.Stable(childSorter{children: ns.children, parentID: ns.ID})
	}

	// Phase 1: topological sort (oldest-first rows).
	st.sort()

	// Phase 2: column assignment (newest-first for branch continuity).
	st.assignColumns()

	// Phase 3: compact non-mainline cols whose lifetimes don't overlap so
	// short-lived side branches reuse a single lane (e.g. sequential feature
	// branches both render in col 1).
	st.compactColumns()

	// Build oldest-first order slice.
	order := make([]*nodeState, len(st.nodes))
	copy(order, st.nodes)
	sort.Slice(order, func(i, j int) bool { return order[i].row < order[j].row })

	// Phase 4: build lanes per col so we can emit per-lane fork / term
	// staggers and reuse cols across non-overlapping lanes.
	st.buildLanes(order)

	// Phase 4b: detect catch-up edges (non-first-parent edges where the
	// source col stays alive past the parent), allocate routing cols.
	st.detectCrossings(order)

	// Phase 5: row generation.
	rows := st.generateRows(order)

	return LayoutResult{Rows: rows, Columns: st.numCols}
}

// childSorter implements sort.Interface for ordering a parent's children
// without allocating a closure per Layout call.
type childSorter struct {
	children []*nodeState
	parentID string
}

func (s childSorter) Len() int      { return len(s.children) }
func (s childSorter) Swap(i, j int) { s.children[i], s.children[j] = s.children[j], s.children[i] }
func (s childSorter) Less(i, j int) bool {
	a, b := s.children[i], s.children[j]
	aFirst := a.Parents[0] == s.parentID
	bFirst := b.Parents[0] == s.parentID
	if aFirst != bFirst {
		return aFirst
	}
	if a.Date != b.Date {
		return a.Date < b.Date
	}
	return a.ID < b.ID
}

// ------ internal state ------------------------------------------------------------------------------------------------

type nodeState struct {
	*Node
	row      int
	col      int
	children []*nodeState
}

type layoutState struct {
	idx        map[string]*nodeState
	nodes      []*nodeState
	numCols    int
	lanes      [][]lane // lanes per col, sorted by introRow
	crossings  map[*nodeState]map[string]int
	glyphArena []Glyph // flat backing for per-row Glyph slices
	arenaOff   int
}

// nextRowGlyphs hands out a numCols-wide sub-slice of the pre-allocated
// glyph arena. All slots come back zero-initialized (= GlyphSpace) since
// the arena was zero-initialized on allocation.
func (st *layoutState) nextRowGlyphs() []Glyph {
	g := st.glyphArena[st.arenaOff : st.arenaOff+st.numCols : st.arenaOff+st.numCols]
	st.arenaOff += st.numCols
	return g
}

// lane is one continuous span of a column's life. A col can host multiple
// disjoint lanes when sequential side branches reuse the same col.
type lane struct {
	col      int
	introRow int // integer row where lane becomes active (= parent.row + 1, or commit row if no parent / lane already in use earlier)
	endRow   int // last integer row of lane activity (last commit row in lane)
	introCol int // col of introducing parent (-1 if no parent / first lane in col 0)
	consumer *nodeState
}

// ------ phase 1: topological sort --------------------------------------------------------------------------

// TODO: support --topo-order.

func (st *layoutState) sort() {
	n := len(st.nodes)
	if n == 0 {
		return
	}

	// indegree: count of children not yet placed.
	indeg := make(map[string]int, n)
	for _, ns := range st.nodes {
		indeg[ns.ID] = len(ns.children)
	}

	// Ready set: nodes whose children are all placed (tips first).
	ready := make([]*nodeState, 0)
	for _, ns := range st.nodes {
		if len(ns.children) == 0 {
			ready = append(ready, ns)
		}
	}

	placed := make(map[string]bool, n)
	row := n - 1 // assign rows newest-first

	// walk places ns and recurses into non-first-parent ancestors immediately,
	// so second-parent branches appear right after the merge. first-parent
	// continuations go through the ready queue keyed by date.
	var walk func(ns *nodeState)
	walk = func(ns *nodeState) {
		if placed[ns.ID] {
			return
		}
		placed[ns.ID] = true
		ns.row = row
		row--

		// Non-first parents: process depth-first, immediately. This places
		// side-branch commits in rows above the merge commit in newest-first
		// order (= just below merge in oldest-first output).
		for i := 1; i < len(ns.Parents); i++ {
			pid := ns.Parents[i]
			p := st.idx[pid]
			if p == nil {
				continue
			}
			indeg[p.ID]--
			if indeg[p.ID] == 0 && !placed[p.ID] {
				walk(p)
			}
		}

		// First parent: also recurse depth-first when ready, so the mainline
		// chain stays contiguous and lands above the side branch in
		// oldest-first output. Falls through to ready queue when not yet
		// reachable (other children still pending).
		if len(ns.Parents) > 0 {
			pid := ns.Parents[0]
			p := st.idx[pid]
			if p != nil {
				indeg[p.ID]--
				if indeg[p.ID] == 0 && !placed[p.ID] {
					walk(p)
				}
			}
		}
	}

	for len(ready) > 0 {
		// Pick newest ready node (date-ordered queue).
		sort.Slice(ready, func(i, j int) bool { return ready[i].Date > ready[j].Date })
		ns := ready[0]
		ready = ready[1:]
		walk(ns)
	}

	// Assign rows to any remaining unplaced nodes (valid DAG should never hit this).
	for _, ns := range st.nodes {
		if !placed[ns.ID] {
			ns.row = row
			row--
		}
	}
}

// ------ phase 2: column assignment --------------------------------------------------------------------

// Newest-first: children assigned before parents, enabling branch
// continuity via first-child inheritance. LayoutHint takes priority
// over child continuity for column-matching.

type columnInfo struct {
	hint string
}

func (st *layoutState) assignColumns() {
	colInfo := []columnInfo{}

	sorted := make([]*nodeState, len(st.nodes))
	copy(sorted, st.nodes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].row > sorted[j].row })

	for _, ns := range sorted {
		st.assignOne(ns, colInfo)
		for ns.col >= len(colInfo) {
			colInfo = append(colInfo, columnInfo{})
		}
		if ns.LayoutHint != "" {
			colInfo[ns.col].hint = ns.LayoutHint
		}
	}
}

func (st *layoutState) assignOne(ns *nodeState, colInfo []columnInfo) {
	// Child continuity wins over hint matching: a node with first-parent
	// children always belongs on the mainline lane that descends from it.
	// Hint matching is for tips and side-branch heads where no first-parent
	// child anchors the col yet.
	bestCol := -1
	for _, child := range ns.children {
		if child.col < 0 {
			continue
		}
		if len(child.Parents) > 0 && child.Parents[0] != ns.ID {
			continue
		}
		if bestCol == -1 || child.col < bestCol {
			bestCol = child.col
		}
	}
	if bestCol >= 0 {
		ns.col = bestCol
		return
	}

	if ns.LayoutHint != "" {
		for c, ci := range colInfo {
			if ci.hint == ns.LayoutHint {
				ns.col = c
				return
			}
		}
	}

	ns.col = st.numCols
	st.numCols++
}

// ------ phase 3: column compaction ------------------------------------------------------

// compactColumns merges non-mainline cols whose commit-row ranges don't
// overlap so sequential side branches share a lane. col 0 is left alone --
// it represents the mainline and always extends across the full history.
func (st *layoutState) compactColumns() {
	if st.numCols < 2 {
		return
	}
	// Compaction uses a narrower activity set than render: only rows with
	// actual commits (plus same-col vertical edges between them). Fork and
	// termination extension rows are excluded so two short-lived side
	// branches off a shared root can share a lane even though their
	// fork-extensions overlap visually pre-compaction.
	activity := make([]map[int]bool, st.numCols)
	for c := 0; c < st.numCols; c++ {
		activity[c] = map[int]bool{}
	}
	for _, ns := range st.nodes {
		activity[ns.col][ns.row] = true
		if len(ns.Parents) > 0 {
			if p := st.idx[ns.Parents[0]]; p != nil && p.col == ns.col {
				for r := p.row + 1; r < ns.row; r++ {
					activity[ns.col][r] = true
				}
			}
		}
	}
	mergedTo := make([]int, st.numCols)
	for i := range mergedTo {
		mergedTo[i] = i
	}
	hasCommit := make([]bool, st.numCols)
	for _, ns := range st.nodes {
		hasCommit[ns.col] = true
	}
	overlaps := func(a, b map[int]bool) bool {
		if len(a) > len(b) {
			a, b = b, a
		}
		for r := range a {
			if b[r] {
				return true
			}
		}
		return false
	}
	for c := 1; c < st.numCols; c++ {
		if mergedTo[c] != c || !hasCommit[c] {
			continue
		}
		for cp := 0; cp < c; cp++ {
			if mergedTo[cp] != cp || !hasCommit[cp] {
				continue
			}
			if !overlaps(activity[c], activity[cp]) {
				mergedTo[c] = cp
				for r := range activity[c] {
					activity[cp][r] = true
				}
				break
			}
		}
	}
	for _, ns := range st.nodes {
		ns.col = mergedTo[ns.col]
	}
	used := map[int]bool{}
	for _, ns := range st.nodes {
		used[ns.col] = true
	}
	cols := make([]int, 0, len(used))
	for c := range used {
		cols = append(cols, c)
	}
	sort.Ints(cols)
	remap := make(map[int]int, len(cols))
	for i, c := range cols {
		remap[c] = i
	}
	for _, ns := range st.nodes {
		ns.col = remap[ns.col]
	}
	st.numCols = len(cols)
}

// ------ phase 4: lane construction ------------------------------------------------------------------------

// buildLanes walks commits in row order and groups them into per-col lanes.
// A lane spans a contiguous run of rows in one col. New lane starts when
// the commit's first parent is in a different col (or has no parent), or
// when a previous lane in the same col already ended.
//
// laneIntroRow defaults to parent.row+1 (so vertical pipes draw between
// parent and first commit) but is pushed later when an earlier lane in
// the same col is still active.
func (st *layoutState) buildLanes(order []*nodeState) {
	st.lanes = make([][]lane, st.numCols)

	// Map node -> index of lane that contains it.
	nodeLane := make(map[string]int)
	// For each col, index of the currently-open lane, or -1.
	openIdx := make([]int, st.numCols)
	for c := range openIdx {
		openIdx[c] = -1
	}

	for _, ns := range order {
		c := ns.col
		extend := false
		var introCol int = -1
		introRow := ns.row
		if len(ns.Parents) > 0 {
			if p := st.idx[ns.Parents[0]]; p != nil {
				if p.col == c {
					extend = true
				} else {
					introCol = p.col
					introRow = p.row + 1
				}
			}
		}
		if extend && openIdx[c] >= 0 {
			st.lanes[c][openIdx[c]].endRow = ns.row
			nodeLane[ns.ID] = openIdx[c]
			continue
		}
		// Start new lane. Push introRow past any prior lane in this col.
		if openIdx[c] >= 0 {
			prevEnd := st.lanes[c][openIdx[c]].endRow
			if introRow <= prevEnd {
				introRow = prevEnd + 1
			}
		}
		// Multi-step forks (col distance > 1) collapse the lane to a single
		// row (the commit row) and emit d stagger rows in the gap right
		// above. Otherwise the destination col would render a stray pipe
		// at the rows between parent and commit.
		if introCol >= 0 {
			d := c - introCol
			if d < 0 {
				d = -d
			}
			if d > 1 {
				introRow = ns.row
			}
		}
		l := lane{col: c, introRow: introRow, endRow: ns.row, introCol: introCol}
		st.lanes[c] = append(st.lanes[c], l)
		openIdx[c] = len(st.lanes[c]) - 1
		nodeLane[ns.ID] = openIdx[c]
	}

	// Mark consumers: for each merge, link non-first-parent lanes to the
	// merge as their consumer.
	for _, ns := range order {
		for i := 1; i < len(ns.Parents); i++ {
			pid := ns.Parents[i]
			p := st.idx[pid]
			if p == nil {
				continue
			}
			li, ok := nodeLane[pid]
			if !ok {
				continue
			}
			if p.col != ns.col {
				st.lanes[p.col][li].consumer = ns
			}
		}
	}
}

// detectCrossings finds non-first-parent edges that cross an active lane
// and allocates a routing col for each. The catch-up edge will be drawn
// as a fork from the source col out to the routing col, then a
// termination from the routing col into the destination col.
func (st *layoutState) detectCrossings(order []*nodeState) {
	st.crossings = map[*nodeState]map[string]int{}
	for _, ns := range order {
		for i := 1; i < len(ns.Parents); i++ {
			pid := ns.Parents[i]
			p := st.idx[pid]
			if p == nil || p.col == ns.col {
				continue
			}
			// Find p's lane in p.col.
			var pLane *lane
			for li := range st.lanes[p.col] {
				l := &st.lanes[p.col][li]
				if l.introRow <= p.row && l.endRow >= p.row {
					pLane = l
					break
				}
			}
			if pLane == nil || pLane.endRow <= p.row {
				continue
			}
			// Source col stays alive past p -- need routing col.
			routingCol := st.numCols
			st.numCols++
			if st.crossings[ns] == nil {
				st.crossings[ns] = map[string]int{}
			}
			st.crossings[ns][pid] = routingCol
		}
	}
}

// ------ phase 5: row generation ------------------------------------------------------------------------

func (st *layoutState) generateRows(order []*nodeState) []Row {
	if len(order) == 0 {
		return nil
	}

	lastRow := order[len(order)-1].row

	// Pre-count total rows (commits + stagger rows) so we can allocate a
	// single flat backing array for all per-row Glyph slices. Each
	// fork/term/crossing emits a known number of stagger rows based on
	// col distance.
	totalRows := len(order)
	for c := 0; c < len(st.lanes); c++ {
		for _, l := range st.lanes[c] {
			if l.introCol < 0 {
				continue
			}
			d := l.col - l.introCol
			if d < 0 {
				d = -d
			}
			totalRows += d
		}
	}
	for _, ns := range order {
		for i := 1; i < len(ns.Parents); i++ {
			p := st.idx[ns.Parents[i]]
			if p == nil || p.col == ns.col {
				continue
			}
			if rc, ok := st.crossings[ns][ns.Parents[i]]; ok {
				d1 := rc - p.col
				if d1 < 0 {
					d1 = -d1
				}
				d2 := rc - ns.col
				if d2 < 0 {
					d2 = -d2
				}
				totalRows += d1 + d2
			} else {
				d := ns.col - p.col
				if d < 0 {
					d = -d
				}
				totalRows += d
			}
		}
	}
	if totalRows > 0 && st.numCols > 0 {
		st.glyphArena = make([]Glyph, totalRows*st.numCols)
	}

	// Active: row is in some lane's [introRow, endRow] span in col c.
	active := func(row int, c int) bool {
		if c < 0 || c >= len(st.lanes) {
			return false
		}
		for _, l := range st.lanes[c] {
			if row >= l.introRow && row <= l.endRow {
				return true
			}
		}
		return false
	}

	// Index commits by row. Rows are dense [0, lastRow] so a slice indexed
	// by row beats a map[int][]*nodeState on alloc count.
	commitsAt := make([][]*nodeState, lastRow+1)
	for _, ns := range order {
		commitsAt[ns.row] = append(commitsAt[ns.row], ns)
	}

	var rows []Row
	for rowNum := 0; rowNum <= lastRow; rowNum++ {
		// Lane introductions at this row: a lane in col c with introRow == rowNum
		// AND introCol >= 0 (i.e., forks from another col, not a root).
		// Iterate only commit cols; routing cols (beyond len(st.lanes)) have
		// no lanes.
		for c := 0; c < len(st.lanes); c++ {
			for _, l := range st.lanes[c] {
				if l.introRow != rowNum || l.introCol < 0 {
					continue
				}
				rows = append(rows, st.forkRows(l, rowNum, active)...)
			}
		}

		// Termination staggers: for each commit at rowNum, each non-first
		// parent at a different col contributes a termination edge. Crossing
		// edges (where the source col stays alive past the parent) route via
		// a temp col -- emitted as a fork to the routing col plus a
		// termination from it to the commit's col.
		for _, ns := range commitsAt[rowNum] {
			for i := 1; i < len(ns.Parents); i++ {
				p := st.idx[ns.Parents[i]]
				if p == nil || p.col == ns.col {
					continue
				}
				if rc, ok := st.crossings[ns][ns.Parents[i]]; ok {
					rows = append(rows, st.crossingStagger(p, ns, rc, rowNum, active)...)
				} else {
					rows = append(rows, st.termRows(p, ns, rowNum, active)...)
				}
			}
		}

		// Commit rows.
		for _, ns := range commitsAt[rowNum] {
			glyphs := st.nextRowGlyphs()
			for c := 0; c < st.numCols; c++ {
				if c == ns.col {
					glyphs[c] = GlyphStar
				} else if active(rowNum, c) {
					glyphs[c] = GlyphPipe
				} else {
					glyphs[c] = GlyphSpace
				}
			}
			rows = append(rows, Row{Commit: ns.Node, Glyphs: glyphs})
		}
	}

	// Trim trailing empty rows.
	for len(rows) > 0 && isEmptyRow(rows[len(rows)-1], st.numCols) {
		rows = rows[:len(rows)-1]
	}
	return rows
}

// forkRows renders the stagger row(s) introducing lane l. For col distance
// d > 1 (cross-routing), emits d rows: each step moves the diagonal one
// col closer to l.col. The destination col is pre-rendered as a vertical
// pipe in earlier steps so the line "appears" at l.col before the diagonal
// physically arrives -- this is git's `|\|` then `| |\` pattern.
func (st *layoutState) forkRows(l lane, rowNum int, active func(int, int) bool) []Row {
	dir := 1
	glyph := GlyphBackslash
	if l.introCol > l.col {
		dir = -1
		glyph = GlyphSlash
	}
	d := l.col - l.introCol
	if d < 0 {
		d = -d
	}
	rows := make([]Row, 0, d)
	for i := 0; i < d; i++ {
		stepCol := l.introCol + dir*(i+1)
		glyphs := st.nextRowGlyphs()
		for c := 0; c < st.numCols; c++ {
			switch {
			case c == stepCol:
				glyphs[c] = glyph
			case c == l.introCol:
				glyphs[c] = GlyphPipe
			case c == l.col:
				glyphs[c] = GlyphPipe
			case active(rowNum, c) || active(rowNum-1, c):
				glyphs[c] = GlyphPipe
			default:
				glyphs[c] = GlyphSpace
			}
		}
		rows = append(rows, Row{Glyphs: glyphs})
	}
	return rows
}

// crossingStagger renders a catch-up termination via a routing col: a
// fork from p.col out to routingCol, then a term from routingCol back to
// ns.col. Used when the source col stays alive past p so the natural
// `|/` would collide with the source col's vertical lane.
func (st *layoutState) crossingStagger(p, ns *nodeState, routingCol, rowNum int, active func(int, int) bool) []Row {
	rows := []Row{}
	forkLane := lane{col: routingCol, introCol: p.col, introRow: rowNum, endRow: rowNum}
	rows = append(rows, st.forkRows(forkLane, rowNum, active)...)
	fakeP := &nodeState{col: routingCol}
	rows = append(rows, st.termRows(fakeP, ns, rowNum, active)...)
	return rows
}

// termRows renders the stagger row(s) terminating an edge from parent p
// into commit ns when they sit on different cols. The diagonal glyph
// sits at the parent's col on step 0 and steps toward ns.col on later
// steps; this matches git's `|/` (or `|\`) at the dying col.
func (st *layoutState) termRows(p, ns *nodeState, rowNum int, active func(int, int) bool) []Row {
	if p.col == ns.col {
		return nil
	}
	dir := 1
	glyph := GlyphBackslash
	if p.col > ns.col {
		dir = -1
		glyph = GlyphSlash
	}
	d := ns.col - p.col
	if d < 0 {
		d = -d
	}
	rows := make([]Row, 0, d)
	for i := 0; i < d; i++ {
		stepCol := p.col + dir*i
		glyphs := st.nextRowGlyphs()
		for c := 0; c < st.numCols; c++ {
			switch {
			case c == stepCol:
				glyphs[c] = glyph
			case c == p.col || c == ns.col:
				glyphs[c] = GlyphPipe
			case active(rowNum, c) || active(rowNum-1, c):
				glyphs[c] = GlyphPipe
			default:
				glyphs[c] = GlyphSpace
			}
		}
		rows = append(rows, Row{Glyphs: glyphs})
	}
	return rows
}

// ------ helpers ------------------------------------------------------------------------------------------------------------

func isEmptyRow(r Row, numCols int) bool {
	if r.Commit != nil {
		return false
	}
	for c := 0; c < numCols; c++ {
		if r.Glyphs[c] != GlyphSpace {
			return false
		}
	}
	return true
}
