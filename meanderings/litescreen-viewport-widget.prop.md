---
status: open
date: 2026-05-05
description: scrollable viewport widget (line-buffer + tail mode + key bindings) as a litescreen sibling pkg
---

# proposal: litescreen viewport widget

## gap

`gg tree --follow` ended up implementing a generic "log tailer" inside
`runFollow`:

- buffer of lines
- scroll offset
- "tail mode" flag -- auto-stick to bottom on update
- key bindings: `j`/`k` line, PgDn/PgUp / Ctrl-D/Ctrl-U / space page,
  `g`/Home top, `G`/End bottom + re-engage tail, `q`/Esc/Ctrl-C exit

this shape generalises. anything that streams or polls a long list of
lines into the screen wants the same widget:

- `gg log --tail` (hypothetical)
- `gg blame` long-file viewer
- future `gg ps` / pipeline / activity views
- generic "stream of stderr from a command, scrollable"

leaving it inline in `runFollow` means the next caller copies + diverges
+ accumulates per-caller bugs.

## options

### where it lives

- **`src/litescreen/viewport`** -- sibling pkg, mirrors `litescreen/ansi`
  layout. imports `litescreen` for the screen type. consumers opt in.
- **`src/litescreen/widget/viewport`** -- nest under a `widget`
  parent so we have room for future widgets (input box, status line,
  tabs) without flat-pkg sprawl. trade off: deeper import path.
- **`src/internal/viewport`** -- if we don't want to commit to "this is
  reusable across consumers outside gg yet", keep internal. trivial to
  promote later.

### scope of the widget

- **read-only line viewport** -- scroll over a fixed `[]string`. caller
  pushes lines; widget renders. minimal.
- **"tail" mode aware** -- knows about auto-stick-to-bottom on update.
  matches the `--follow` need exactly.
- **key handling included** -- widget consumes a `*tcell.EventKey`,
  mutates state, returns whether it handled the key (so unrecognised
  keys bubble up to caller). vs caller-owned key dispatch with widget
  exposing `ScrollUp()`/`ScrollDown()`/etc. methods.
- **rendering included** -- widget knows its own (x, y, w, h) region
  and draws into a `*litescreen.Screen`. vs widget produces a
  `[]string` slice and caller draws.

likely shape: state holder + draw method + key handler. caller still
owns the loop.

### ansi awareness

- **plain text only** -- caller pre-strips. simple.
- **ansi-aware** -- viewport stores raw ANSI strings, calls
  `litescreen/ansi.DrawString` per line. matches `gg tree` need
  (git log --color=always). depends on the ansi-bounded-draw proposal.

likely yes -- ansi support is the whole reason this came up.

### multi-region layout

- **viewport occupies a rect** -- caller specifies `(x, y, w, h)`. widget
  draws within. lets caller compose viewport + header + footer.
- **viewport assumes whole screen minus N rows** -- simpler API but
  rigid. callers w/ richer layouts work around it.

rect is more general; one extra param.

## sketch

```go
// src/litescreen/viewport/viewport.go
package viewport

import (
    "github.com/gdamore/tcell/v2"
    "github.com/lczyk/gitgum/src/litescreen"
    "github.com/lczyk/gitgum/src/litescreen/ansi"
)

type Viewport struct {
    Lines    []string // ansi-aware, one per row
    Offset   int      // first visible line index
    TailMode bool     // stick to bottom on SetLines
}

// SetLines replaces the line buffer. If TailMode, scroll snaps to
// the new bottom on next Draw.
func (v *Viewport) SetLines(lines []string) {
    v.Lines = lines
}

// Draw renders the viewport into scr at rect (x, y, w, h). Clips
// internally; safe at any size.
func (v *Viewport) Draw(scr *litescreen.Screen, x, y, w, h int) {
    visible := max(h, 0)
    maxOffset := max(len(v.Lines)-visible, 0)
    if v.TailMode { v.Offset = maxOffset }
    v.Offset = max(0, min(v.Offset, maxOffset))
    end := min(v.Offset+visible, len(v.Lines))
    for i, line := range v.Lines[v.Offset:end] {
        ansi.DrawString(scr, x, y+i, line, tcell.StyleDefault)
    }
}

// HandleKey applies a navigation event. Returns true if the key was
// handled. Page size derives from the viewport's last drawn height
// (caller passes it; or store it from last Draw).
func (v *Viewport) HandleKey(ev *tcell.EventKey, page int) bool { ... }
```

## tradeoffs

- **adds a pkg** -- one more import path. modest. mitigated by clear
  separation (viewport is opinionated; core stays minimal).
- **opinionated key bindings** -- widget bakes `j`/`k`/`g`/`G`/etc.
  consumers may want different bindings. mitigation: expose primitive
  methods (`Up()`, `Down()`, `PageUp()`, `Top()`, `BottomTail()`) and
  let `HandleKey` be a convenience that calls them; consumers w/ custom
  bindings skip `HandleKey` and call primitives directly.
- **state ownership** -- widget owns `Offset` / `TailMode`. caller
  reaches in to read / set. could expose getters/setters but go
  conventions say bare exported fields are fine for plain data.
- **page-size source** -- widget needs to know viewport height for
  page-nav math. options: cache from last Draw, or caller passes
  explicitly to HandleKey. cache is more ergonomic; explicit is more
  testable.

## open questions

- pkg name: `viewport` / `pager` / `tailer`? `viewport` is generic;
  `pager` implies less / more semantics; `tailer` overloads with
  log-tailing connotation. probably `viewport`.
- single line buffer (`[]string`) or `io.Reader`-driven? caller-owned
  buffer is simpler; reader-driven would let the widget run async.
  likely defer reader-driven to v2.
- shared scroll state across multiple widget instances (split panes)?
  YAGNI; one viewport per screen for now.
- mouse scroll support? out of scope for v1; tcell event types support
  it but litescreen's input parser may not yet.
- search (`/` then incremental) -- big feature. separate proposal if
  ever needed.
- highlighting current line / selection? would turn this from "viewer"
  into "picker"; fuzzyfinder already covers picker semantics. keep
  viewport read-only.

## touched files (if landed)

- `src/litescreen/viewport/viewport.go` (new, ~120 lines)
- `src/litescreen/viewport/viewport_test.go` (new, ~150 lines --
  state transitions are pure-func; easy to unit test)
- `src/commands/tree.go` (drop scroll state + `handleFollowKey`,
  delegate to widget)

## dependencies on other proposals

- depends on `litescreen-ansi-bounded-draw.prop.md` for the per-line
  ansi draw call. could degrade to plain text if that one isn't
  landed first, but ansi support is the use case.
- parallels `litescreen-bounded-draw.prop.md` (header rendering still
  done by caller; viewport doesn't draw headers).
