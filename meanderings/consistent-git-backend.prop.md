---
status: open
date: 2026-05-02
description: route every git invocation through one helper that neutralises user config so output is parse-stable
---

# proposal: consistent git backend

## gap

gg shells out to `git` and parses stdout / stderr. user gitconfig and
env can perturb that output in ways that silently break parsing:

- `color.ui=always` -> ansi escapes inside porcelain.
- `core.quotepath=true` (default) -> non-ascii paths come back octal-
  escaped (`"\303\244.txt"`).
- `i18n.logOutputEncoding`, `LANG`, `LC_ALL` -> error strings shift
  language. existing string-match in `internal/git/git.go` keys off the
  literal `"no upstream configured for branch"`; under `LANG=de_DE` it
  silently returns the wrong path.
- `status.relativePaths=false` -> absolute paths in `status` porcelain
  break the tree builder.
- `log.showSignature=true` -> extra `gpg: ...` lines in `git log` break
  oneline parsers.
- pager (`core.pager`) -> already worked around in one spot
  (`status.go:28` uses `--no-pager`); inconsistent elsewhere.
- aliases -- inert for us today b/c we only invoke real subcommands,
  but worth keeping that property intentional rather than incidental.

call sites are also inconsistent. most go through `Repo.run` ->
`cmdrun.RunIn`, but:

- `src/commands/status.go` calls `cmdrun.Run("git", ...)` directly
  three times.
- `internal/git/git.go:61` `GetFileStatus` builds an `exec.Command`
  manually to dodge `cmdrun.Run`'s whitespace trim.

so even if we harden `Repo.run`, those bypass paths stay raw.

scope: **reads and writes**. writes (commit, branch, push, etc.)
inherit user identity (`user.name`, `user.email`, hooks, signing) and
need different treatment, but the call-site routing is the same
problem. one helper, two profiles.

## options

### chokepoint shape

- **single helper, two profiles**: `runRead` and `runWrite`. shared
  prelude (`--no-pager`, locale lock, `-c color.ui=never`); read profile
  also drops global/system config. construction stays in one place.
- **single helper, opts struct**: `run(args, runOpts{write: true,
  ...})`. flexible but every caller picks options; drift over time.
- **single helper, always-strict**: same env for read and write. risks
  surprising users on writes (their `commit.gpgsign`, hooks dir,
  template, etc. would vanish). rejected on principle -- writes should
  honour the user's git identity.

### config neutralisation

- **`-c key=value` per-invocation**: surgical, lists exactly what we
  depend on. drift risk -- new git versions add knobs we don't pin.
- **`GIT_CONFIG_GLOBAL=/dev/null` + `GIT_CONFIG_SYSTEM=/dev/null`**:
  blanket ignore of user/system config. repo-local `.git/config`
  still honoured (needed for remotes, upstream, worktree config). drops
  the user's aliases, color, pager, locale prefs in one shot. cannot
  use for writes -- kills `user.name` / `user.email` / signing.
- **both, layered**: env strips global+system, `-c` re-asserts the few
  knobs we explicitly want even at repo level (e.g. `color.ui=never` in
  case the repo's own `.git/config` set `color.ui=always`).

### locale

- **`LC_ALL=C`** in the helper env. forces ascii error strings, makes
  `strings.Contains(stderr, "no upstream configured for branch")` and
  similar matches stable. no downside for parsing pipelines.
- alternative: replace string-matching with exit-code / `--porcelain`
  inspection where possible. orthogonal cleanup, doable later.

### whitespace trimming

`cmdrun.Run` trims trailing whitespace, which `GetFileStatus` needs
preserved. options:

- `cmdrun.RunRaw` variant that doesn't trim. callers opt in.
- helper returns `(stdoutRaw, stdoutTrimmed, stderr, err)`. clunky.
- per-call flag on the helper. simplest if the helper is the only
  entry point anyway.

### `runWrite` profile

write profile keeps user identity but still locks parse-stable knobs:

- env: `LC_ALL=C` only (no `GIT_CONFIG_*` blanking). preserves
  `user.email`, signing config, hooks dir, commit template.
- `-c`: `color.ui=never`, `core.quotepath=false`, `--no-pager`.
- explicitly does **not** set `commit.gpgsign=false` or override
  `core.hooksPath` -- user's signing / hook setup stands.

two profiles only -- no third "no-hooks" profile. user hooks fire by
default on writes; gg behaves like git. for the rare op where firing
hooks would be wrong because gg is using git as plumbing rather than
expressing user intent, opt out per-call:

```go
r.runWrite("-c", "core.hooksPath=/dev/null", "stash", "push", ...)
```

confirmed cases needing the opt-out:

- **stash** -- gg uses stash internally only (e.g. switch_stream
  bookkeeping). user `pre-stash` hooks assume "user is parking work";
  firing them on gg's plumbing lets the user's hook reject gg's own
  ops. `stash pop` / `apply` also re-trigger `post-checkout`, doubling
  side effects (tag refresh, dep install) per save/pop pair.
- (future) any other internal-only plumbing op -- e.g. throwaway
  worktrees, scratch branches.

ops that *do* express user intent (`commit`, `checkout` of a real
branch the user named, `switch`) keep hooks on -- that's the whole
point of two profiles instead of three.

## sketch

```go
// internal/git/run.go (new)
package git

import (
    "os/exec"
    // ...
)

var readPrelude = []string{
    "--no-pager",
    "-c", "color.ui=never",
    "-c", "core.quotepath=false",
    "-c", "status.relativePaths=true",
    "-c", "log.showSignature=false",
}

var writePrelude = []string{
    "--no-pager",
    "-c", "color.ui=never",
    "-c", "core.quotepath=false",
}

func (r Repo) runRead(args ...string) (string, string, error) {
    full := append(append([]string{}, readPrelude...), args...)
    return runWith(r.Dir, readEnv(), full...)
}

func (r Repo) runWrite(args ...string) (string, string, error) {
    full := append(append([]string{}, writePrelude...), args...)
    return runWith(r.Dir, writeEnv(), full...)
}

func readEnv() []string {
    return append(os.Environ(),
        "GIT_CONFIG_GLOBAL=/dev/null",
        "GIT_CONFIG_SYSTEM=/dev/null",
        "LC_ALL=C",
    )
}

func writeEnv() []string {
    return append(os.Environ(), "LC_ALL=C")
}
```

`runWith` wraps `exec.Command` directly so the whitespace-trim
question is decided in one place (recc: don't trim, return raw, let
each caller `strings.TrimSpace` if it wants -- removes the
`GetFileStatus` special case).

## principles to follow when writing new git callers

guidance for any code that consumes git output, beyond just routing
through the helpers:

- **prefer exit codes + porcelain over stderr-string matches.** in
  order of preference:
  1. dedicated exit code (`ls-remote --exit-code`,
     `diff --quiet`/`--exit-code`).
  2. porcelain / structured stdout (`status --porcelain`,
     `branch --format`, `for-each-ref --format`, `rev-list`).
  3. `-z` null-delimited output for paths -- defends against
     newlines / quotes in filenames when paired w/ porcelain.
  4. stderr string match -- last resort. comment must name the git
     version the phrasing was confirmed against
     (`// confirmed: git 2.43.0`); add a test that fails fast if
     phrasing drifts.
  rationale: `LC_ALL=C` locks the locale axis but not the version
  axis -- upstream git rephrases user-facing strings between
  releases. tracked separately as
  `structured-git-error-detection.prop.md`.
- **don't trim stdout in the helper.** caller decides whether
  trailing whitespace is signal (porcelain) or noise (single-line
  refs). lets each caller `strings.TrimSpace` if it wants.
- **always pass `ctx`.** even if the caller only has
  `context.Background()` today, threading it now means future
  cancellation work doesn't churn every signature.

## migration

- migrate every `Repo.run` call to `Repo.runRead` / `Repo.runWrite`
  based on side-effect.
- migrate three direct `cmdrun.Run("git", ...)` sites in
  `src/commands/status.go` to `Repo.runRead`.
- migrate the `exec.Command` in `internal/git/git.go:61` to
  `Repo.runRead` once trimming is per-caller.
- audit `src/commands/*.go` for any remaining direct `cmdrun.Run`
  with `"git"` as argv0.
- delete the old `Repo.run` once empty.

writes to migrate (non-exhaustive, audit pass needed): `commit`,
`branch -d/-D`, `push`, `checkout`, `switch`, `stash`, `tag`,
`rebase`, `merge`, `reset`. each touched in `src/commands/*.go`.

## tests

- `internal/git/run_test.go`:
  - poison test env: `GIT_CONFIG_GLOBAL` set to a temp config with
    `color.ui=always`, `core.quotepath=true`, alias `status =
    log`. assert `runRead("status", "--porcelain")` returns
    unescaped, uncolored, and runs the real `status` (alias bypassed
    because we invoke real subcommands).
  - poison `LANG=de_DE.UTF-8`, run a command that errors -> assert
    stderr contains the english string we match against.
  - `runWrite` w/ poisoned global config -> still strips color, but
    keeps `user.email` from the temp global if set (assert identity
    preserved).
- existing parsers unchanged -- regression coverage already there.

## resolved

- **windows**: deferred. `GIT_CONFIG_GLOBAL=/dev/null` works
  linux/macos; windows uses `NUL`. revisit when gg targets windows.
- **locale + passthrough**: passthrough commands leave locale alone
  (no `LC_ALL=C`). long-term goal is to remove passthrough entirely
  -- gg should always parse git output and re-render, never relay raw
  bytes. once that lands, every read can use `LC_ALL=C` uniformly.
- **repo-local `color.ui=always`**: handled in prelude via
  `-c color.ui=never` -- repo-level config is overridden too.
- **write profile granularity**: two profiles, not three. `runWrite`
  always honours user hooks + identity; ops where hooks would be
  wrong (gg-as-plumbing) opt out per-call via
  `-c core.hooksPath=/dev/null`. confirmed initial case: internal
  stash. see `runWrite profile` section above.
- **chokepoint enforcement**: import-graph fence. all
  `exec.Command("git", ...)` and any git invocation lives inside
  `internal/git`. `cmdrun` keeps non-git callers but no longer
  accepts `"git"` as argv0 (or simply doesn't get used for git).
  reviewers + future callers can't regress because the affordance is
  removed at the package boundary.
- **repo-pointer env vars**: strip `GIT_DIR`, `GIT_WORK_TREE`,
  `GIT_INDEX_FILE`, `GIT_COMMON_DIR` on both read and write profiles.
  gg always identifies its target via `Repo.Dir` / `-C`; inherited
  pointers from parent shells / processes risk gg writing to the
  wrong repo. set each to empty string in env builder (git treats
  empty as not-set).
- **`GIT_TERMINAL_PROMPT=0`**: set unconditionally on both profiles.
  gg is non-interactive plumbing -- never wants a TTY cred prompt.
  failure mode shifts from "hang forever" to clear error.
- **`GIT_TRACE` / `GIT_TRACE_*` family**: strip on both profiles.
  user-set tracing corrupts parsed stderr; ok to lose trace output
  on writes too -- if user really wants traced writes, they can
  invoke git directly. enumerate set in env builder (`GIT_TRACE`,
  `GIT_TRACE_PACKET`, `GIT_TRACE_PACK_ACCESS`,
  `GIT_TRACE_PERFORMANCE`, `GIT_TRACE_SETUP`, `GIT_TRACE_CURL`,
  `GIT_TRACE2`, `GIT_TRACE2_EVENT`, `GIT_TRACE2_PERF`).
- **`GIT_PAGER`**: strip (empty) on both profiles. belt for
  `--no-pager`; old git consults env ahead of `core.pager`.
- **`GIT_ASKPASS` / `SSH_ASKPASS` / `GIT_EDITOR`**: not our problem.
  `GIT_TERMINAL_PROMPT=0` short-circuits the git-side cred prompt;
  `SSH_ASKPASS` only fires under unusual ssh setups; `GIT_EDITOR`
  only fires on ops gg doesn't issue interactively (gg always passes
  `-m` for commit/tag, never invokes `rebase -i`).
- **`safe.directory`**: blanking `GIT_CONFIG_GLOBAL` kills user's
  safe-dir allowlist; git 2.35.2+ then refuses repos owned by a
  different UID (shared boxes, sudo-cloned repos, docker bind
  mounts, CI). add `-c safe.directory=<r.Dir>` per-call (surgical;
  not blanket `*`). requires `r.Dir` to be absolute -- canonicalise
  in `Repo` constructor or `runRead` / `runWrite`.
- **submodules**: deferred. gg isn't currently designed or tested
  for repos containing submodules. gap documented in `README.md`
  under "Known gaps". revisit when submodule support becomes a
  goal -- at that point, decide whether to set
  `-c submodule.recurse=false` for parse-stable reads, and whether
  writes should respect or override user's global recurse pref.
- **`-C <dir>` vs `cmd.Dir`**: `-C r.Dir` in argv when `r.Dir != ""`,
  drop `cmd.Dir`. explicit in `ps` / strace output when debugging a
  stuck git invocation; functionally equivalent otherwise.
- **context / cancellation**: bake in now. signatures take
  `ctx context.Context` as first arg --
  `runRead(ctx, args...)` / `runWrite(ctx, args...)` -- backed by
  `exec.CommandContext`. callers w/o real ctx pass
  `context.Background()` explicitly; ugly but greppable when
  retrofit comes. propagates SIGKILL on cancel, lets gg kill stuck
  children when user `Ctrl-C`s or picker session ends early.
  retrofit later means churning every `Repo` method signature; one-
  time tax now is cheaper.
- **stderr capture vs passthrough**: separate variant. default
  `runRead` / `runWrite` always capture stderr (parsed errors / hide
  routine noise). `runWriteStreaming` (and `runReadStreaming` if
  needed) for the handful of network ops -- `fetch`, `push`,
  `clone` -- that emit live progress users want to see. avoids opts-
  struct bloat at the 95% of call sites that don't care.
- **stderr-string matching even at `LC_ALL=C`**: separate proposal.
  `LC_ALL=C` solves locale axis but not version axis -- english
  strings shift between git releases. tracked as
  `structured-git-error-detection.prop.md`. principle: prefer exit
  codes + `--porcelain` markers; string match is last resort.
- **min git version floor**: 2.35.2 (Mar 2022). lower bound driven
  by `safe.directory` (2.35.2+); `GIT_CONFIG_GLOBAL` (2.32+) and
  `for-each-ref --format='%(upstream:short)'` (2.13+) below it.
  startup check: parse `git --version` once on first `runRead` /
  `runWrite`, cache, fail loudly w/ clear message if below floor.
  document in `README.md`. one syscall per gg invocation; cheap
  insurance vs cryptic fallout from missing config knobs.
- **alternative backend (libgit2 / go-git)**: deferred indefinitely.
  considered: `go-git` (pure Go, no cgo) is the more attractive
  option; `libgit2` adds cgo build pain. switching kills most of
  this proposal's surface (env stripping, `-c` preludes, version
  floor) and removes subprocess overhead. trades that for: gg loses
  bit-identical git semantics, has to reimplement signing
  (`gpg.program` shell-out, canonical commit/tag object format,
  three signature backends, ~500-1000 LOC w/ subtle silent-failure
  modes) and the `git-credential-<helper>` protocol (~200-300 LOC,
  but long tail of yubikey/ssh-agent/oauth/smartcard integrations
  subprocess gets free). a quarter to a half of the win evaporates
  into reimplementing what subprocess inherits. gg's value is the
  UX layer, not the git engine -- not worth multiplying scope.
  revisit only if subprocess overhead becomes a measurable bottleneck
  (it isn't today).

## open questions

env-stripping gaps (read profile in particular):

config gaps:

shape questions:

other:

(none currently.)
