# todo

## graph: native vs git divergences

scanned across multiple repos (chisel, pebble, rocks-security-manifest,
gitgum) for cases where native rendering is *incorrect* (broken
topology, dangling edges, glyph sequences that don't represent valid
graph structure) vs merely stylistically different from git --graph.

### resolved

- shared-parent dual-merge topo ordering (4d3f68e)
- catch-up routing reuses idle col (6ca87cb)
- collapse pushed lane intro to commit row (d8ccc4b) -- was producing
  `|\` immediately followed by `|/` in sequential side-branch merges,
  with the new col still drawn alive at the merge's commit row.

### remaining (stylistic, not incorrect)

- native compacts non-mainline cols more aggressively than git. for
  long-lived "phantom" lanes (a side-branch lane open through several
  merges before its merge-back), git keeps the lane visible as `|`
  across the span; native drops it once no commit lives in it. both are
  valid graph encodings; native's rendering is narrower.

- pebble: ~140-line block of commits placed in different relative
  position than git. valid topo ordering in both; engine prefers
  depth-first via first-parent chains, git uses date-priority queue.
  worth revisiting if a user explicitly wants git's ordering.

no current cases of *broken* topology or dangling glyphs in native
output across the scanned repos.
