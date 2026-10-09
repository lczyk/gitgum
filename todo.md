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

## push: follow-ups

left out of the push transactional work, mostly because they reach into
code other commands share.

- auto-stash pops whatever is on top of the stash list (StashPopIndex),
  not the entry it made. refs/stash is shared across worktrees, so a
  stash made in another terminal mid-operation gets popped instead. pop
  by the created stash's sha. shared by every command that auto-stashes.
- discard at the dirty-tree prompt stays destructive (reset --hard +
  clean -fd) even when push later undoes its rebase, so the discarded
  changes can't come back. make it a `stash -u` dropped only once the
  operation has succeeded; the row's "cannot be undone" wording changes
  with it. the prompt is shared.
- branches that push somewhere other than their upstream (pushRemote /
  remote.pushDefault, @{push} != @{u}) are still compared against the
  upstream while a plain push goes to @{push}. compare against @{push}.
- remote names containing '/' aren't handled: ruledOut and the upstream
  split cut on the first '/' (the assumption RemoteBranches documents).
- bring the other commands in line with AGENTS.md's interactive rules:
  - delete: the local branch goes before the remote one; a failed remote
    delete doesn't restore it.
  - checkout-pr: WriteMeta runs after ResetHard, with no undo or hint if
    it fails.
  - release: the version-mention picker runs after the auto-stash.
  - switch / branch: pull's prompts run after the checkout, so cancelling
    them leaves you switched.
  - empty / release: decide whether the commit is undone when the push /
    tag after it fails (today it's kept; release prints an undo).
- a Ctrl-C landing just as a step in push succeeds can be missed:
  interrupts.seen has no grace period for a trailing signal, unlike
  interrupts.or, so the next step can go ahead. rare; adding one costs a
  wait after every successful step.
- abortRebase leans on `git rebase --abort`, which git's own docs say an
  ill-timed Ctrl-C can defeat under the apply backend (rebase.backend=apply
  or an old git). the merge backend, git's default, is fine.
