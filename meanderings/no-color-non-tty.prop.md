---
status: open
date: 2026-05-01
description: gate ansi escapes on NO_COLOR + stdout-is-tty so `watch -n1 gg tree` stops mangling
---

# proposal: NO_COLOR + non-tty detection

## gap

`gg status` (tree + headers) always emits ansi escapes. piping to
non-tty consumers (e.g. `watch -n1 gg tree`, `gg status | less` w/out
`-R`, ci logs) shows raw `\033[...m` sequences. existing TODO at
`src/commands/status_tree.go:64`.

two emitters:

- `src/commands/status_tree.go` (tree rendering, lines 65-120)
- `src/commands/status.go:20` (`printHeader`)

both must gate.

## detection rule

`colorEnabled()` true iff **all** hold:

- `os.Getenv("NO_COLOR") == ""` (per no-color.org -- any non-empty value
  disables)
- `os.Stdout` is char device:
  `fi, _ := os.Stdout.Stat(); fi.Mode()&os.ModeCharDevice != 0`

stdlib only -- no `golang.org/x/term` dep.

out of scope for v1: `FORCE_COLOR`, `CLICOLOR` / `CLICOLOR_FORCE`,
`--color=auto|always|never` flag. `watch -c` is the workaround until
`FORCE_COLOR` lands.

## options

### where the helper lives

- **new `src/commands/color.go`** -- co-located w/ only consumers, no
  new pkg. simplest.
- **new `internal/term` pkg** -- room to grow (width detection, raw
  mode, etc.). overkill for one func atm.

### detection cadence

- **`sync.Once` cache** -- one syscall per process. but breaks tests
  that toggle `NO_COLOR` via `t.Setenv` mid-run.
- **recompute per call** -- one stat + env lookup; cheap relative to
  the rendering allocs already happening per node. testable.
- **cache + `resetColorCache()` test hook** -- middle ground, more
  surface area.

### plumbing vs global

detection keys off `os.Stdout`, not the `io.Writer` arg threaded
through `renderTree(w)`. tests pass `&bytes.Buffer{}` but the helper
still inspects real stdout.

- **global helper** -- trivial; matches prod (commands always write to
  stdout). tests force-disable via `t.Setenv("NO_COLOR", "1")`.
- **plumb `useColor bool`** -- testable per-call, but touches every
  signature (`renderTree`, `renderNode`, `formatLeaf`, `colorCode`,
  `colorCodeChar`, `dim`, `printHeader`). a lot of churn for one knob.

## sketch

```go
// src/commands/color.go
package commands

import "os"

const ansiBlack = "\033[0;30m"

func colorEnabled() bool {
    if os.Getenv("NO_COLOR") != "" {
        return false
    }
    fi, err := os.Stdout.Stat()
    if err != nil {
        return false
    }
    return fi.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
    if !colorEnabled() {
        return s
    }
    return code + s + ansiReset
}
```

refactor:

- `status_tree.go`: drop standalone `dim()`; route every `ansiX + s +
  ansiReset` through `paint(ansiX, s)`. `colorCodeChar`, `colorCode`,
  `formatLeaf`, `renderNode`, `renderTree` all go through `paint`.
- `status.go:20` `printHeader`: `fmt.Fprintln(out, paint(ansiBlack,
  msg))`.
- remove TODO at `status_tree.go:64`.

## tests

- existing `status_tree_test.go` strips ansi via regex -- passes
  regardless of color state.
- new `color_test.go`:
  - `NO_COLOR=1` -> `colorEnabled()` false, `paint` returns input
    verbatim.
  - `NO_COLOR=""` under `go test` (stdout is pipe) -> false. exercises
    non-tty branch.
- integration: `renderTree` w/ `NO_COLOR` set, assert no `\x1b` in
  output.

## touched files

- `src/commands/color.go` (new, ~25 lines)
- `src/commands/status_tree.go` (refactor `dim` / branches to `paint`)
- `src/commands/status.go` (gate `printHeader`)
- `src/commands/color_test.go` (new)

## open questions

- detection cadence: cache vs recompute. recompute leans simpler +
  testable; cache is premature opt.
- ship `FORCE_COLOR` in the same change, or wait for a real ask?
- should `gg status --flat` also gate the `printHeader` calls? (yes --
  same `out`, same problem.)
