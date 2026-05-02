---
status: open
date: 2026-05-03
description: replace stderr-string matching with exit codes + porcelain markers so error detection survives git version bumps
---

# proposal: structured git error detection

## gap

gg detects some error conditions by string-matching the stderr of
`git`. even with `LC_ALL=C` locked in (see
`consistent-git-backend.prop.md`), the english strings still shift
between git versions -- the upstream source has rephrased many user-
facing messages across releases. a phrasing change in a future git
release silently flips gg's branch behaviour: an `if
strings.Contains(stderr, "no upstream configured for branch")` that
previously returned `("", "", nil)` starts returning the raw error
instead, propagating to the CLI as a confusing failure.

`LC_ALL=C` solves the *locale* axis. it does not solve the
*version* axis.

## current findings

audit of stderr string matches in non-test code (as of 2026-05-03):

- `internal/git/git.go:129` -- `GetBranchUpstream`,
  `strings.Contains(stderr, "no upstream configured for branch")`.
- `internal/git/git.go:191` -- `GetCurrentBranchUpstream`, same
  string.

both detect the "branch has no upstream" case for
`git rev-parse --abbrev-ref --symbolic-full-name <ref>@{u}`. on
success, git prints the upstream ref to stdout and exits 0. on no-
upstream, git exits non-zero and writes the english error to stderr.

no other stderr string matches found in `src/` or `internal/`
(non-test). small surface; cheap to fix.

## options

### per-site replacement

for the two upstream-check sites, two structurally-better signals:

- **`git for-each-ref --format='%(upstream)' refs/heads/<branch>`**
  -- always exits 0 when the branch exists. stdout is the upstream
  ref name (e.g. `refs/remotes/origin/main`) or empty string if no
  upstream. no stderr parsing. branch-doesn't-exist still surfaces
  via empty output + exit 0, which is a different (and currently
  unhandled) case worth distinguishing.
- **keep `rev-parse @{u}` but key off exit code**: git exits 128 for
  the no-upstream case. risk: 128 is git's generic "fatal" exit, so
  same code covers other errors (missing branch, malformed ref).
  needs supplementary check (stdout empty? stderr matched against a
  *list* of known phrasings?) to disambiguate -- which puts us back
  near string matching.

`for-each-ref` is the clean win. exit codes alone aren't enough for
this particular call.

### general principle

going forward, prefer signals in this order:

1. **dedicated exit code** (`ls-remote --exit-code` exits 2 on no-
   match; `diff --quiet` / `diff --exit-code`).
2. **porcelain / structured stdout** (`status --porcelain`,
   `branch --format`, `for-each-ref --format`, `rev-list`).
3. **`-z` null-delimited output** for paths -- defends against
   newlines / quotes in filenames when paired w/ porcelain.
4. **stderr string match** -- last resort, only when nothing else
   works. comment must name the git version the phrasing was
   confirmed against, e.g. `// confirmed: git 2.43.0`. add a test
   that runs against the local git and fails fast if the phrasing
   has drifted.

## sketch

```go
// internal/git/git.go
func (r Repo) GetBranchUpstream(branch string) (remote, remoteBranch string, err error) {
    stdout, _, err := r.runRead(
        "for-each-ref",
        "--format=%(upstream:short)",
        "refs/heads/"+branch,
    )
    if err != nil {
        return "", "", err
    }
    if stdout == "" {
        return "", "", nil
    }
    parts := strings.SplitN(stdout, "/", 2)
    if len(parts) != 2 {
        return "", "", fmt.Errorf("unexpected upstream format: %s", stdout)
    }
    return parts[0], parts[1], nil
}
```

`GetCurrentBranchUpstream` collapses to two calls: `GetCurrentBranch`
followed by `GetBranchUpstream`. or stays distinct and uses
`HEAD`-resolved ref directly.

## tests

- table-driven test: branch w/ upstream, branch w/o upstream, branch
  that doesn't exist. assert each case maps to the right return.
- (optional) version-canary test: assert
  `git rev-parse @{u}` against a no-upstream branch *does* still
  produce stderr containing the phrase we used to match. if it
  doesn't, the canary fires before any real user is bitten -- not
  required, but cheap insurance for the principle going forward.

## touched files

- `internal/git/git.go` -- replace both call sites.
- `internal/git/git_test.go` -- table tests for new behaviour.

## open questions

- branch-doesn't-exist: today both call sites silently treat any non-
  upstream-string error as a real error. `for-each-ref` flips this:
  unknown branch -> empty output -> we return `nil` upstream w/ no
  error, indistinguishable from "branch exists but has no upstream".
  do we need to disambiguate, or do callers already not care?
- worth adding a lightweight `errors.Is`-style typed error
  (`ErrNoUpstream`, `ErrUnknownBranch`) so callers can branch on
  category rather than checking nil/non-nil + empty-string?
- version-canary tests: aspirational discipline or too-niche to be
  worth the upkeep?
