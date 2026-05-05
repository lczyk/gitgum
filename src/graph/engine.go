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

	for i := range g.Nodes {
		n := &g.Nodes[i]
		ns := &nodeState{Node: n, row: -1, col: -1}
		st.idx[n.ID] = ns
		st.nodes = append(st.nodes, ns)
	}

	// Build child→parent reverse edges.
	for _, ns := range st.idx {
		for _, pid := range ns.Parents {
			if p, ok := st.idx[pid]; ok {
				p.children = append(p.children, ns)
			}
		}
	}
	// Sort children deterministically: first-parent children first (mainline
	// continuity), then by date, then by ID. Map iteration above randomizes
	// append order; without this every phase that iterates children is unstable.
	for _, ns := range st.nodes {
		sort.SliceStable(ns.children, func(i, j int) bool {
			a, b := ns.children[i], ns.children[j]
			aFirst := a.Parents[0] == ns.ID
			bFirst := b.Parents[0] == ns.ID
			if aFirst != bFirst {
				return aFirst
			}
			if a.Date != b.Date {
				return a.Date < b.Date
			}
			return a.ID < b.ID
		})
	}

	// Phase 1: topological sort (oldest-first rows).
	st.sort()

	// Phase 2: column assignment (newest-first for branch continuity).
	st.assignColumns()

	// Phase 3: fix sibling conflicts at merges.
	st.fixMergeSiblings()

	// Phase 3b: compact non-mainline cols whose lifetimes don't overlap so
	// short-lived side branches reuse a single lane (e.g. sequential feature
	// branches both render in col 1).
	st.compactColumns()

	// Build oldest-first order slice.
	order := make([]*nodeState, len(st.nodes))
	copy(order, st.nodes)
	sort.Slice(order, func(i, j int) bool { return order[i].row < order[j].row })

	// Phase 4: column lifetimes, then row generation.
	rows := st.generateRows(order)

	return LayoutResult{Rows: rows, Columns: st.numCols}
}

// ------ internal state ------------------------------------------------------------------------------------------------

type nodeState struct {
	*Node
	row      int
	col      int
	children []*nodeState
}

type layoutState struct {
	idx     map[string]*nodeState
	nodes   []*nodeState
	numCols int
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
	if ns.LayoutHint != "" {
		// Match existing column claimed by this hint.
		for c, ci := range colInfo {
			if ci.hint == ns.LayoutHint {
				ns.col = c
				return
			}
		}
		// No matching col yet. If a first-parent child has a col, inherit
		// that (mid-history nodes carry hints from their branch but should
		// stay on the mainline lane rather than starting a fresh col).
		// Tip nodes with no first-parent child fall through to allocate.
	}

	// Child continuity: among first-parent children (those that descend from
	// ns via mainline), pick the lowest col so ns stays on the canonical
	// lane. Fall back to any child if no first-parent child has a col yet.
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

	// New column. Reached when ns has no first-parent child with an
	// assigned col -- typical for branch tips and side-branch nodes whose
	// only descendant is a merge that doesn't continue the lane.
	ns.col = st.numCols
	st.numCols++
}

// ------ phase 3: fix merge siblings ----------------------------------------------------------------------

// If a merge commit has two parents in the same column (because both
// inherited from the merge child), give the second parent its own column.
// Walk oldest->newest so the earlier sibling keeps the shared column.
//
// When a parent is moved, cascade the change up single-parent chains so
// the entire branch stays in one column.

func (st *layoutState) fixMergeSiblings() {
	sorted := make([]*nodeState, len(st.nodes))
	copy(sorted, st.nodes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].row < sorted[j].row })

	for _, ns := range sorted {
		if len(ns.Parents) < 2 {
			continue
		}
		seen := map[int]bool{}
		for _, pid := range ns.Parents {
			p, ok := st.idx[pid]
			if !ok || p.col < 0 {
				continue
			}
			if seen[p.col] {
				// Conflict: two parents share a column. Give this parent a new one.
				oldCol := p.col
				p.col = st.numCols
				st.numCols++
				st.cascadeColumn(p, oldCol, p.col)
			}
			seen[p.col] = true
		}
	}
}

// cascadeColumn walks up the ancestor chain (following parents) and moves
// nodes from oldCol to newCol. it stops at forks (multiple children) and
// LayoutHint anchors.
func (st *layoutState) cascadeColumn(ns *nodeState, oldCol, newCol int) {
	for _, pid := range ns.Parents {
		p := st.idx[pid]
		if p == nil || p.LayoutHint != "" || p.col != oldCol {
			continue
		}
		if len(p.children) == 1 {
			p.col = newCol
			st.cascadeColumn(p, oldCol, newCol)
		}
	}
}

// ------ phase 3b: column compaction ------------------------------------------------------

// compactColumns merges non-mainline cols whose commit-row ranges don't
// overlap so sequential side branches share a lane. col 0 is left alone --
// it represents the mainline and always extends across the full history.
func (st *layoutState) compactColumns() {
	if st.numCols < 2 {
		return
	}
	// Build per-col activity (rows where a vertical line / commit exists)
	// using current col assignments. Two cols can share a lane iff their
	// activity sets are disjoint -- this is stricter than min/max overlap
	// because it accounts for fork-extension rows above the first commit.
	order := make([]*nodeState, len(st.nodes))
	copy(order, st.nodes)
	sort.Slice(order, func(i, j int) bool { return order[i].row < order[j].row })
	activity := st.columnActivity(order)
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

// ------ phase 4: column activity ------------------------------------------------------------------

// columnActivity returns, for each col, the set of integer rows at which a
// vertical line is drawn. A row is active in col c if (a) a commit lives at
// (c, row), (b) a vertical edge passes through (c, row) between a commit
// and its same-col first parent, or (c) a diagonal edge from another col
// terminates into / forks out of col c at row.
//
// Tracking active rows as a sparse set (instead of a [min, max] interval)
// is what lets sequential side branches share a lane without drawing a
// connecting pipe through the merge that separates them.
func (st *layoutState) columnActivity(order []*nodeState) []map[int]bool {
	active := make([]map[int]bool, st.numCols)
	for c := 0; c < st.numCols; c++ {
		active[c] = map[int]bool{}
	}
	for _, ns := range order {
		active[ns.col][ns.row] = true
		// Vertical span between ns and its first parent when both share a col.
		if len(ns.Parents) > 0 {
			if p := st.idx[ns.Parents[0]]; p != nil {
				if p.col == ns.col {
					for r := p.row + 1; r < ns.row; r++ {
						active[ns.col][r] = true
					}
				} else {
					// Fork: col ns.col is introduced at p.row+1 and stays
					// active down to ns (vertical line between fork point
					// and first commit on this lane).
					for r := p.row + 1; r < ns.row; r++ {
						active[ns.col][r] = true
					}
				}
			}
		}
		// Termination of a non-first parent's col at this merge.
		for i := 1; i < len(ns.Parents); i++ {
			p := st.idx[ns.Parents[i]]
			if p == nil || p.col == ns.col {
				continue
			}
			// p's col stays active down to one row above ns.
			for r := p.row + 1; r < ns.row; r++ {
				active[p.col][r] = true
			}
		}
	}
	return active
}

// ------ phase 5: row generation ------------------------------------------------------------------------

func (st *layoutState) generateRows(order []*nodeState) []Row {
	if len(order) == 0 {
		return nil
	}

	activeMap := st.columnActivity(order)
	lastRow := order[len(order)-1].row

	active := func(row int, c int) bool {
		if c < 0 || c >= len(activeMap) {
			return false
		}
		return activeMap[c][row]
	}

	var rows []Row
	commitIdx := 0

	for rowNum := 0; rowNum <= lastRow; rowNum++ {
		for commitIdx < len(order) && order[commitIdx].row == rowNum {
			ns := order[commitIdx]
			commitIdx++

			// Pre-stagger: terminating cols (parents of ns at different cols).
			rows = append(rows, st.buildStagger(ns, rowNum, active)...)

			// Commit row.
			glyphs := make([]Glyph, st.numCols)
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

			// Post-stagger: fork rows for children at different cols whose
			// lane is introduced from ns.
			rows = append(rows, st.buildForkStagger(ns, rowNum, active)...)
		}
	}

	// Trim trailing empty rows.
	for len(rows) > 0 && isEmptyRow(rows[len(rows)-1], st.numCols) {
		rows = rows[:len(rows)-1]
	}
	return rows
}

// ------ stagger rows ------------------------------------------------------------------------------------------------

// buildForkStagger emits stagger rows immediately after ns's commit row for
// each child whose first-parent is ns but whose col differs from ns.col.
// These are the lane-introduction diagonals (`|\`) that git draws right
// below a parent commit when its branch splits.
func (st *layoutState) buildForkStagger(ns *nodeState, rowNum int, active func(int, int) bool) []Row {
	var forkCols []int
	for _, child := range ns.children {
		if len(child.Parents) == 0 || child.Parents[0] != ns.ID {
			continue
		}
		if child.col == ns.col {
			continue
		}
		forkCols = append(forkCols, child.col)
	}
	if len(forkCols) == 0 {
		return nil
	}
	glyphs := make([]Glyph, st.numCols)
	for c := 0; c < st.numCols; c++ {
		isFork := false
		for _, fc := range forkCols {
			if fc == c {
				isFork = true
				break
			}
		}
		switch {
		case c == ns.col:
			glyphs[c] = GlyphPipe
		case isFork:
			if c > ns.col {
				glyphs[c] = GlyphBackslash
			} else {
				glyphs[c] = GlyphSlash
			}
		case active(rowNum+1, c):
			glyphs[c] = GlyphPipe
		default:
			glyphs[c] = GlyphSpace
		}
	}
	return []Row{{Glyphs: glyphs}}
}

func (st *layoutState) buildStagger(ns *nodeState, baseRow int, active func(int, int) bool) []Row {
	if len(ns.Parents) == 0 {
		return nil
	}
	if len(ns.Parents) > 2 {
		return nil // skip octopus
	}

	type pinfo struct {
		id  string
		col int
	}
	var parents []pinfo
	for _, pid := range ns.Parents {
		if ps, ok := st.idx[pid]; ok && ps.col >= 0 {
			parents = append(parents, pinfo{pid, ps.col})
		}
	}
	if len(parents) == 0 {
		return nil
	}
	if len(parents) == 1 {
		// Single-parent forks now handled post-commit by buildForkStagger.
		return nil
	}
	return st.staggerMerge(ns.col, parents[0].col, parents[1].col, baseRow, active)
}

func (st *layoutState) staggerSingle(src, dst, baseRow int, active func(int, int) bool) []Row {
	if src == dst {
		return nil
	}
	dir := 1
	glyph := GlyphBackslash
	if src > dst {
		dir = -1
		glyph = GlyphSlash
	}
	dist := dst - src
	if dist < 0 {
		dist = -dist
	}

	var rows []Row
	cur := src
	for range dist {
		next := cur + dir
		// Diagonal sits at the higher-numbered col; pipe at the lower
		// (mainline-side) col. Matches `git log --graph` convention.
		diagCol, straightCol := next, cur
		if cur > next {
			diagCol, straightCol = cur, next
		}
		glyphs := make([]Glyph, st.numCols)
		for c := 0; c < st.numCols; c++ {
			if c == diagCol {
				glyphs[c] = glyph
			} else if c == straightCol || active(baseRow+len(rows), c) {
				glyphs[c] = GlyphPipe
			} else {
				glyphs[c] = GlyphSpace
			}
		}
		rows = append(rows, Row{Glyphs: glyphs})
		cur = next
	}
	return rows
}

func (st *layoutState) staggerMerge(commitCol, p1Col, p2Col, baseRow int, active func(int, int) bool) []Row {
	// Stagger drawn before merge row: parents sit ABOVE (older rows) and
	// terminate INTO ns (commit) below. Flow direction is parent.col → ns.col
	// downward. Pass parent col first so the glyph (/ or \) matches the
	// down-flow visual.
	if p1Col == commitCol {
		return st.staggerSingle(p2Col, commitCol, baseRow, active)
	}
	if p2Col == commitCol {
		return st.staggerSingle(p1Col, commitCol, baseRow, active)
	}
	d1 := p1Col - commitCol
	if d1 < 0 {
		d1 = -d1
	}
	d2 := p2Col - commitCol
	if d2 < 0 {
		d2 = -d2
	}
	farCol := p1Col
	if d2 > d1 {
		farCol = p2Col
	}
	return st.staggerSingle(farCol, commitCol, baseRow, active)
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
