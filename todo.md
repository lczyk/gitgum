# todo

## graph: native vs git divergences

### 1. lane over-compaction in catch-up merge regions (chisel)

native compacts a non-mainline col/lane more aggressively than git. when a
fork opens a lane at row X, no commit lives in that lane until row Y > X
(lane is "passing through" toward a later merge), and another commit at
row Z (X < Z < Y) is at the mainline col, native drops the side-lane
during the [X, Z] span and re-introduces it later. git keeps the side
lane alive across the whole [X, Y] span.

repro: chisel repo, commits b72322a..bf2a16e region.

```
git:                       native:
* b72322a                  * b72322a
|\                         * eeba444
* | eeba444                * 45c7083
* | 45c7083                * 49010d4
* | 49010d4                * f0bff5a
* | f0bff5a                |\
|\|                        | * 91c8f04
| |\                       * bf2a16e
| | * 91c8f04
|\ \
* | | bf2a16e
```

likely cause: `buildLanes` / `compactColumns` consider only commit-rows
plus same-col first-parent vertical pipe rows as "active". fork-to-merge
spans where the lane is just a passing-through `|` are not counted, so a
sequential mainline commit can grab/share the col.

investigation start: src/graph/engine.go `compactColumns` (line ~349) and
`buildLanes` (line ~441).

### 2. topo-order divergence on long histories (pebble)

native walk produces a substantially different commit ordering than git
for long histories with many merges. ~140-line block of commits placed
in an entirely different position. likely a depth-first vs date-priority
heap difference: engine recurses depth-first on first-parent and
non-first-parent chains, git uses a date-priority queue with topo
constraints.

repro: pebble repo, ~line 125 of `gg tree --since=''` output diverges
massively from git mode.

investigation start: src/graph/engine.go `sort` (line ~182). may need to
pop from a date-ordered ready queue for first-parent continuations, not
just for tip selection.
