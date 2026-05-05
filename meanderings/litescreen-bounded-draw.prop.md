---
status: open
date: 2026-05-05
description: bounded `DrawString` helper in litescreen core to replace per-caller clip + setcontent loops
---

# proposal: litescreen `DrawString` helper

## gap

every consumer of `litescreen.Screen` that writes plain text ends up
re-implementing the same loop:

```go
w, h := scr.Size()
x := x0
for _, r := range s {
    if x >= w || y < 0 || y >= h { break }
    scr.SetContent(x, y, r, nil, style)
    x++
}
```

first instance landed in `src/commands/tree.go` (`writePlain`) for the
follow-mode header + error line. fuzzyfinder has its own equivalents
inline. anything else that treats the screen like "draw a styled string at
(x, y)" repeats this.

## options

### where it lives

- **`litescreen.DrawString(scr *Screen, x, y int, s string, style tcell.Style)`** --
  package-level free function. minimal coupling, no method bloat on
  `Screen`. matches `tcell.Screen`'s style of free helpers.
- **`(*Screen).DrawString(x, y int, s string, style tcell.Style)`** --
  method form. slightly more discoverable (autocomplete on screen var),
  but locks it to litescreen's screen type only -- can't pass a
  `tcell.SimulationScreen` adapter.

### return value

- **none** -- silent clip, simplest API.
- **`(endX int)`** -- final x cursor position after writing. lets callers
  chain (`x = DrawString(...); DrawString(scr, x, y, " more")`).
- **`(endX, written int)`** -- both cursor + glyph count. extra signal
  for callers that care about runes-emitted vs runes-clipped.

### bounds

- **`>= w` clips the rune** -- standard.
- **what about wide runes (CJK / emoji)?** runewidth says some glyphs are
  width 2. `SetContent` handles that internally? unclear -- need to read
  `framebuf.set`. for v1 may be fine to ignore (clip is approximate); but
  worth checking before calling this "general purpose".

### newlines

- **plain helper does not interpret `\n`** -- caller passes single line,
  bumps y manually for multi-line.
- **interpret `\n`** -- jump to (x0, y+1). matches what `runFollow` does
  in its scroll loop. but adds an x0 implicit-state surprise.

current consumer in `tree.go` already handles \n at the slice-and-loop
level (one DrawString per line); so single-line-only is fine.

## sketch

```go
// litescreen/draw.go (new file)
package litescreen

import "github.com/gdamore/tcell/v2"

// DrawString writes runes from s into scr at (x, y), advancing rightwards.
// Clips at the screen's right edge; silently no-ops if y is out of bounds.
// Returns the column position after the last written rune (or x if y was
// out of bounds).
func DrawString(scr *Screen, x, y int, s string, style tcell.Style) int {
    w, h := scr.Size()
    if y < 0 || y >= h { return x }
    for _, r := range s {
        if x >= w { break }
        scr.SetContent(x, y, r, nil, style)
        x++
    }
    return x
}
```

## tradeoffs

- **api surface** -- one new exported func. small.
- **caller code shrink** -- `tree.go` drops 12 lines (writePlain). other
  future consumers don't write the loop at all.
- **discoverability** -- free func vs method. method form may match
  tcell.Screen ergonomics callers expect.
- **wide-rune correctness** -- punted unless we test it.

## open questions

- free function or method?
- return cursor position or void?
- handle `\n` or single-line only?
- separate variant for right-anchored / centred draws? probably no --
  YAGNI; caller can compute x.

## touched files (if landed)

- `src/litescreen/draw.go` (new, ~25 lines)
- `src/commands/tree.go` (drop `writePlain`, replace 2 call sites)
- maybe future: fuzzyfinder draw loops where the pattern recurs
