---
status: open
date: 2026-05-05
description: bounded `DrawString` in litescreen/ansi that wraps `WriteToScreen` with screen-aware clipping
---

# proposal: ansi-aware bounded `DrawString`

## gap

`litescreen/ansi.WriteToScreen` is geometry-agnostic -- it takes a free
`set(x, y, r, style)` callback. consumers (currently `tree.go`'s
`writeAnsi`) wrap it with an inline clip:

```go
func writeAnsi(scr *litescreen.Screen, x0, y0 int, s string, base tcell.Style, w, h int) {
    set := func(x, y int, r rune, style tcell.Style) {
        if y < 0 || y >= h || x < 0 || x >= w { return }
        scr.SetContent(x, y, r, nil, style)
    }
    ansi.WriteToScreen(set, x0, y0, s, base)
}
```

every caller that wants "draw an ansi-coloured string into a screen"
will write the same wrapper. parallels the plain-text bounded-draw
gap (see `litescreen-bounded-draw.prop.md`).

## options

### where it lives

- **`litescreen/ansi`** -- co-located with the parser. sibling pkg
  imports `litescreen` to know about `*Screen`. circular? no:
  `litescreen` does not import `ansi`. one-way dep is fine.
- **`litescreen` core** -- if we move it here, `litescreen` would import
  `litescreen/ansi`, inverting the dep. bad: pulls SGR parsing into
  every litescreen consumer.

ansi pkg is the right home.

### naming alongside the proposed core helper

if `litescreen.DrawString` exists for plain text and
`litescreen/ansi.DrawString` exists for ansi strings, that's two
`DrawString`s that callers reach for via the import path. not bad but
worth flagging:

- pro: callers swap the import to switch behaviours; both signatures
  identical except for "this one parses ansi".
- con: bare name `DrawString` reads as plain string; ansi-version is a
  silent upgrade. prefer something like `ansi.DrawAnsiString` or
  `ansi.DrawAnsi`?

### signature parity

if the plain version returns `endX int` and handles single-line only,
the ansi version has to decide:

- **single-line only** -- error or undefined behaviour on `\n`. but the
  whole reason `WriteToScreen` exists is multi-line ANSI (git log
  output). single-line ansi is rare; multi-line is the common case.
- **multi-line** -- handles `\n` -> next row at x0. matches
  `WriteToScreen`. return `(endX, endY)` so the api signals it
  consumed multiple rows.

if we go multi-line for ansi but single-line for plain, the two
helpers diverge. consistent argument lists across pkgs is nice but the
common case differs -- forcing single-line on the ansi helper would
make every caller split + loop themselves, defeating the point.

acceptable asymmetry. document in package docs.

## sketch

```go
// litescreen/ansi/draw.go (new file)
package ansi

import (
    "github.com/gdamore/tcell/v2"
    "github.com/lczyk/gitgum/src/litescreen"
)

// DrawString parses s as ANSI-styled text and writes runes into scr
// starting at (x, y). Newlines advance to (x0, y+1) where x0 is the
// initial column. Clips at the screen's bounds. Returns final
// (endX, endY).
func DrawString(scr *litescreen.Screen, x, y int, s string, base tcell.Style) (int, int) {
    w, h := scr.Size()
    set := func(cx, cy int, r rune, style tcell.Style) {
        if cy < 0 || cy >= h || cx < 0 || cx >= w { return }
        scr.SetContent(cx, cy, r, nil, style)
    }
    return WriteToScreen(set, x, y, s, base)
}
```

## tradeoffs

- **callers shrink** -- `tree.go`'s `writeAnsi` wrapper goes away.
- **dep direction** -- `ansi` already standalone; this would make it
  import `litescreen` for the `*Screen` type. not free: anyone wanting
  ansi parsing without litescreen still has the parser via `Parse` /
  `WriteToScreen`. only `DrawString` adds the dep. acceptable.
- **alternative: accept an interface** -- define a tiny
  `type CellSetter interface { SetContent(...); Size() (int, int) }`
  in the ansi pkg. avoids the import entirely. tradeoff: one more
  named interface to explain; very minor.

## open questions

- name: `DrawString` (parallel to core helper) or `DrawAnsi` (signal
  difference)?
- accept `*litescreen.Screen` directly or via `CellSetter` interface?
- multi-line semantics: stick with WriteToScreen's `\n` -> (x0, y+1),
  or also handle `\r`?

## touched files (if landed)

- `src/litescreen/ansi/draw.go` (new, ~25 lines)
- `src/commands/tree.go` (drop `writeAnsi` wrapper)

## relation to other proposals

- depends on no other proposal directly, but parallels the plain-text
  bounded-draw helper -- best designed together so the apis stay in
  sync as much as the asymmetry allows.
