---
status: open
date: 2026-05-01
description: ff library draws tree itself from structured parent/child input
---

# proposal: hierarchy-aware ff

## gap

`ff` today takes flat lines. piping pre-rendered hierarchy text (e.g.
`tree | ff`) breaks the glyphs once the matched set is filtered --
remaining `├── └── │` markers no longer line up with the visible
parent/child structure.

want: caller passes structured nodes, ff draws the glyphs at render
time so they always match the visible set. parsing `tree(1)` output is
explicitly _not_ in scope.

## options

### data model

- **flat list of `(id, parent_id, label)` nodes**: easy to mutate
  concurrently, parent-before-child not required, maps cleanly to
  existing `Source.Snapshot` semantics. picker builds child index per
  refresh.
- **nested `Children []*TreeNode`**: more natural to construct, but
  harder to mutate concurrently and harder to map back to flat indices
  for cursor / selection state.

### option surface

- **new `Tree TreeSource` field on `Opt`**, mutually exclusive with
  flat `Source`. `initFinder` branches on which is set.
- **fold into `Source` via a sniff**: less explicit, harder to spot in
  callers.

### filtering

empty query: full tree visible.

non-empty query: match against `label` only.

how to keep hierarchy readable when only a few nodes match?

- **expand match set with ancestors**: matched node + all its
  ancestors visible. ancestors render dimmed; cursor skips them
  (only matched nodes selectable). simple, no collapse state.
- **expand with siblings too**: more context, more noise.
- **collapse / expand keybind**: full interactivity, more code, more
  state to persist across refreshes.

### render

- **glyph prefix computed at draw time** from "is-last-child at depth"
  bitmap; precomputed once per filter result and stored alongside
  `matched` indices. label runes follow prefix; existing highlight +
  truncation logic operates on label range only.
- **cache full prefix string per node**: simpler but stale on filter
  change (siblings shift `├──` <-> `└──`).

### selection output

- **return node `id`**: stable, caller maps back to whatever they
  need.
- **return label-path** (`a/b/c`): convenient for cli but ambiguous
  when labels contain `/`.
- **return both**, caller chooses.

### cli input format

- **tsv `<id>\t<parent_id>\t<label>` per line**: unambiguous, easy to
  generate, blank parent = root.
- **indent-based**: nicer to hand-type, ambiguous if labels contain
  tabs / leading whitespace.
- **json**: most flexible, heaviest to produce from a shell pipeline.

## tradeoffs

- expanding ancestors but not siblings keeps the visible set tight,
  but means a query that hits one deep leaf still draws every
  ancestor -- can dominate the screen for deep trees. live with it
  for v1, revisit if it bites.
- cursor-on-matched-only is simpler but rules out using ff as a
  pure tree browser (no query, navigate). flat-mode behaviour
  unchanged, so this is only a tree-mode constraint.
- id-based selection forces callers to maintain a node table.
  label-path is friendlier for ad-hoc shell use; tsv input already
  asks the caller to invent ids, so adding a path-output flag is
  cheap.

## open questions

- should non-matched ancestors be selectable too? (would let user
  pick a subtree root from a partial match.)
- collapse / expand worth shipping in v1, or after we see how
  expand-on-match feels?
- when a parent_id refers to an unknown node, treat as root + warn,
  or drop the node?
- how to handle ties in label-path output across siblings with the
  same label?

## files likely touched

- [src/fuzzyfinder/source.go](../src/fuzzyfinder/source.go) -- new
  `TreeNode`, `TreeSource`, optional `SliceTreeSource`
- [src/fuzzyfinder/option.go](../src/fuzzyfinder/option.go) -- `Tree`
  field
- [src/fuzzyfinder/fuzzyfinder.go](../src/fuzzyfinder/fuzzyfinder.go)
  -- init branch, filter w/ ancestor expansion, render w/ glyph prefix,
  prefix-aware highlight offset + truncation
- [cmd/fuzzyfinder/main.go](../cmd/fuzzyfinder/main.go) -- input-format
  flag, tsv parser, id output
- tests: tree variants in
  [src/fuzzyfinder/fuzzyfinder_test.go](../src/fuzzyfinder/fuzzyfinder_test.go)
  + cli test
