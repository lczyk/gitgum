# Fuzzyfinder library streamline plan

Series of focused commits. Each chunk lands `make test` green, then commits.
Branch: `work`. No push. Commit format: heredoc with `Co-Authored-By: Claude Opus 4.7`.

## Chunks

### 1. Drop unused options
Remove from `src/fuzzyfinder/option.go` and threaded internals:
- `WithPreviewWindow` + preview pane rendering (`_drawPreview` etc.)
- `WithCursorPosition`
- `WithPreselected` + preselection logic in `initFinder` and `updateItems`

Keep: `WithMode`, `matching.Mode`, `ModeSmart/ModeCaseSensitive/ModeCaseInsensitive` (future fzf parity).

Verify: `make test` green.

### 2. Substring matcher helper
- Add `fuzzyfinder.SubstringMatcher` (word-split, case-insensitive `Contains` per word).
- Replace inline matchers in `src/commands/switch.go` and `internal/ui/ui.go` with the helper.

Verify: `make test` green.

### 3. Default prompt `"> "`
- Set `defaultOption.promptString = "> "` in `src/fuzzyfinder/option.go`.
- Drop redundant `WithPromptString("> ")` calls and equivalent in callers where the new default suffices. Keep custom prompts where they convey context (e.g. "Select a branch to switch to: ").

Verify: `make test` green.

### 4. Drop `interface{}` / `itemFunc` indirection (library-breaking)
- Change signatures:
  - `Find(items []string, opts ...Option) (int, error)`
  - `FindMulti(items []string, opts ...Option) ([]int, error)`
  - `FindLive(items *[]string, lock sync.Locker, opts ...Option) (int, error)`
  - `FindMultiLive(items *[]string, lock sync.Locker, opts ...Option) ([]int, error)`
- Remove `reflect` usage and `itemFunc` parameter.
- Remove `WithHotReloadLock` — its role is now expressed by choosing `FindLive*`.
- Update all callers (`switch.go`, `ui.go`, `cmd/fuzzyfinder/main.go`, library tests).

Verify: `make test` green.

