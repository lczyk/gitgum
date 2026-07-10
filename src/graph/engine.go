package graph

import "sort"

// newLayoutState builds the shared graph state from nodes: the id index, the
// child->parent reverse edges, and a deterministic per-parent child ordering.
// It does not assign rows -- layoutWalker runs the row-walker on top.
func newLayoutState(nodes []Node) *layoutState {
	st := &layoutState{
		idx:   make(map[string]*nodeState, len(nodes)),
		nodes: make([]*nodeState, 0, len(nodes)),
	}

	// Count children per parent ahead of time so we can pre-size each
	// children slice exactly. Saves per-append slice-growth allocs in
	// scenarios where parents have multiple children.
	childCount := make(map[string]int, len(nodes))
	for _, n := range nodes {
		for _, pid := range n.Parents {
			childCount[pid]++
		}
	}

	// Slab-allocate nodeState in one shot, then hand out pointers into it.
	// Replaces N small heap allocs with one. Same trick for the children
	// slices: one shared backing array, each ns gets a zero-length sub-slice
	// with cap == its child count so appends never realloc.
	slab := make([]nodeState, len(nodes))
	totalChildren := 0
	for _, n := range nodes {
		totalChildren += len(n.Parents)
	}
	childSlab := make([]*nodeState, totalChildren)
	slabOff := 0
	for i := range nodes {
		n := &nodes[i]
		ns := &slab[i]
		ns.Node = n
		ns.row = -1
		if c := childCount[n.ID]; c > 0 {
			ns.children = childSlab[slabOff : slabOff : slabOff+c]
			slabOff += c
		}
		if _, dup := st.idx[n.ID]; dup {
			panic("graph: duplicate Node.ID " + n.ID)
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
	// Children lists are tiny (almost always 2-4 entries); a manual stable
	// insertion sort avoids sort.Stable's per-call auxiliary allocations
	// which dominated the alloc count on histories with many sortable
	// groups.
	for _, ns := range st.nodes {
		if len(ns.children) < 2 {
			continue
		}
		insertionSortChildren(ns.children, ns.ID)
	}
	return st
}

// Layout takes a slice of Nodes and returns a layout result describing how
// each commit maps to a (row, col) pair in the rendered output, plus any
// stagger rows for fork / merge / catch-up edges. It runs the active-lanes
// row-walker (walker.go): a single newest-first sweep that keeps one column
// per live edge, so lanes never disconnect.
//
// Pass the zero Opt for the defaults.
func Layout(nodes []Node, opt Opt) LayoutResult {
	if len(nodes) == 0 {
		return LayoutResult{}
	}
	return layoutWalker(nodes, opt)
}

// insertionSortChildren stable-sorts a parent's children slice in place.
// Order: first-parent children first (mainline continuity), then by
// epoch, then label, then ID. Inlined and allocation-free; suitable for
// the tiny slices (2-4 entries typical) that get sorted on every Layout
// call.
func insertionSortChildren(children []*nodeState, parentID string) {
	less := func(a, b *nodeState) bool {
		aFirst := a.Parents[0] == parentID
		bFirst := b.Parents[0] == parentID
		if aFirst != bFirst {
			return aFirst
		}
		if a.Epoch != b.Epoch {
			return a.Epoch < b.Epoch
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		return a.ID < b.ID
	}
	for i := 1; i < len(children); i++ {
		cur := children[i]
		j := i - 1
		for j >= 0 && less(cur, children[j]) {
			children[j+1] = children[j]
			j--
		}
		children[j+1] = cur
	}
}

// ------ internal state ------------------------------------------------------------------------------------------------

type nodeState struct {
	*Node
	row      int
	children []*nodeState
}

type layoutState struct {
	idx   map[string]*nodeState
	nodes []*nodeState
}

// ------ topological sort --------------------------------------------------------------------------

// headDescendants returns the set of node IDs covering the IsHead node and all
// its descendants (reachable via child edges). Returns nil when no node sets
// IsHead, so the caller's ordering is unaffected.
func (st *layoutState) headDescendants() map[string]bool {
	var head *nodeState
	for _, ns := range st.nodes {
		if ns.IsHead {
			head = ns
			break
		}
	}
	if head == nil {
		return nil
	}
	set := map[string]bool{}
	var walk func(ns *nodeState)
	walk = func(ns *nodeState) {
		if set[ns.ID] {
			return
		}
		set[ns.ID] = true
		for _, c := range ns.children {
			walk(c)
		}
	}
	walk(head)
	return set
}

func (st *layoutState) sort(opt Opt) {
	n := len(st.nodes)
	if n == 0 {
		return
	}

	// indegree: count of children not yet placed.
	indeg := make(map[string]int, n)
	for _, ns := range st.nodes {
		indeg[ns.ID] = len(ns.children)
	}

	// headSide is HEAD plus its descendant-closure (the commits that must render
	// below it). Floating these ahead of everything else in the ready queue sinks
	// HEAD to the lowest row the DAG allows -- the bottom row when it's a tip.
	// Empty (no IsHead node, or the caller opted out) leaves ordering untouched.
	var headSide map[string]bool
	if !opt.NoHeadFloat {
		headSide = st.headDescendants()
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

		// Pre-decrement first parent's indeg before descending into non-first
		// parents. When a non-first parent chain reaches a node that's also
		// shared with our first parent (e.g. m7 = outer's first-parent AND
		// inner's second-parent), the descendant's decrement will see the
		// already-reduced count and hit 0, walking the shared chain depth-first
		// from the descendant. Without this pre-decrement, the shared chain
		// gets stranded until our trailing first-parent block runs, by which
		// time the descendant has already walked its own first-parent chain --
		// flipping their relative ordering.
		var fp *nodeState
		if len(ns.Parents) > 0 {
			if p := st.idx[ns.Parents[0]]; p != nil {
				indeg[p.ID]--
				fp = p
			}
		}

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

		// First parent: walk if ready and not already placed by a shared-chain
		// descent above. Falls through to ready queue when not yet reachable
		// (other children still pending).
		if fp != nil && !placed[fp.ID] && indeg[fp.ID] == 0 {
			walk(fp)
		}
	}

	for len(ready) > 0 {
		// Pick newest ready node (date-ordered queue).
		sort.Slice(ready, func(i, j int) bool {
			a, b := ready[i], ready[j]
			if ah, bh := headSide[a.ID], headSide[b.ID]; ah != bh {
				return ah // head-side tips drain first -> sink to the bottom
			}
			if a.Epoch != b.Epoch {
				return a.Epoch > b.Epoch
			}
			if a.Label != b.Label {
				return a.Label < b.Label
			}
			return a.ID < b.ID
		})
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
