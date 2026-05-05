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

	// Phase 1: topological sort (oldest-first rows).
	st.sort()

	// Phase 2: column assignment (newest-first for branch continuity).
	st.assignColumns()

	// Phase 3: fix sibling conflicts at merges.
	st.fixMergeSiblings()

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
	sort.Slice(st.nodes, func(i, j int) bool {
		return st.nodes[i].Date < st.nodes[j].Date
	})

	pos := make(map[string]int, len(st.nodes))
	for i, ns := range st.nodes {
		pos[ns.ID] = i
	}

	// Ensure parent appears before child in oldest-first order.
	changed := true
	for changed {
		changed = false
		for _, ns := range st.nodes {
			for _, child := range ns.children {
				if pos[ns.ID] > pos[child.ID] {
					if pos[child.ID] < pos[ns.ID]+1 {
						pos[child.ID] = pos[ns.ID] + 1
						changed = true
					}
				}
			}
		}
		if changed {
			type pair struct {
				ns *nodeState
				p  int
			}
			tmp := make([]pair, len(st.nodes))
			for i, ns := range st.nodes {
				tmp[i] = pair{ns, pos[ns.ID]}
			}
			sort.Slice(tmp, func(i, j int) bool { return tmp[i].p < tmp[j].p })
			for i, pr := range tmp {
				st.nodes[i] = pr.ns
				pos[pr.ns.ID] = i
			}
		}
	}

	for i, ns := range st.nodes {
		ns.row = i
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
		// No existing column -- start a new one. Don't fall through
		// to child continuity: a named branch keeps its own lane even
		// if children (from merges) are in different columns.
		ns.col = st.numCols
		st.numCols++
		return
	}

	// Child continuity: reuse first child's column.
	for _, child := range ns.children {
		if child.col >= 0 {
			ns.col = child.col
			return
		}
	}

	// New column.
	ns.col = st.numCols
	st.numCols++
}

// ------ phase 3: fix merge siblings ----------------------------------------------------------------------

// If a merge commit has two parents in the same column (because both
// inherited from the merge child), give the second parent its own column.
// Walk oldest→newest so the earlier sibling keeps the shared column.

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
				p.col = st.numCols
				st.numCols++
			}
			seen[p.col] = true
		}
	}
}

// ------ phase 4: column lifetimes ------------------------------------------------------------------

func (st *layoutState) columnLifetimes(order []*nodeState) (minRow, maxRow []int) {
	minRow = make([]int, st.numCols)
	maxRow = make([]int, st.numCols)
	for c := 0; c < st.numCols; c++ {
		minRow[c] = -1
		maxRow[c] = -1
	}
	for _, ns := range order {
		c := ns.col
		if minRow[c] == -1 || ns.row < minRow[c] {
			minRow[c] = ns.row
		}
		if ns.row > maxRow[c] {
			maxRow[c] = ns.row
		}
	}
	return
}

// ------ phase 5: row generation ------------------------------------------------------------------------

func (st *layoutState) generateRows(order []*nodeState) []Row {
	if len(order) == 0 {
		return nil
	}

	minRow, maxRow := st.columnLifetimes(order)
	lastRow := order[len(order)-1].row

	// A column is active from its newest entry (smallest row) to its
	// oldest entry (largest row), inclusive, with no gaps. This keeps
	// branch lines continuous across the full tree height.
	active := func(row int, c int) bool {
		return minRow[c] >= 0 && maxRow[c] >= minRow[c] && row >= minRow[c] && row <= maxRow[c]
	}

	var rows []Row
	commitIdx := 0

	for rowNum := 0; rowNum <= lastRow; rowNum++ {
		for commitIdx < len(order) && order[commitIdx].row == rowNum {
			ns := order[commitIdx]
			commitIdx++

			// Stagger rows route from ns.col to parent columns.
			stagger := st.buildStagger(ns, rowNum, active)
			rows = append(rows, stagger...)

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
		}
	}

	// Trim trailing empty rows.
	for len(rows) > 0 && isEmptyRow(rows[len(rows)-1], st.numCols) {
		rows = rows[:len(rows)-1]
	}
	return rows
}

// ------ stagger rows ------------------------------------------------------------------------------------------------

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
		return st.staggerSingle(ns.col, parents[0].col, baseRow, active)
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
		glyphs := make([]Glyph, st.numCols)
		for c := 0; c < st.numCols; c++ {
			if c == cur {
				glyphs[c] = glyph
			} else if c == next || active(baseRow+len(rows), c) {
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
	if p1Col == commitCol {
		return st.staggerSingle(commitCol, p2Col, baseRow, active)
	}
	if p2Col == commitCol {
		return st.staggerSingle(commitCol, p1Col, baseRow, active)
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
	return st.staggerSingle(commitCol, farCol, baseRow, active)
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
