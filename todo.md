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

## switch: follow-ups

found while chasing `gg switch` onto a same-name branch of another remote
(local `main` tracked `canonical/main`, picked `playground/main`, diverged
1 ahead / 25 behind). the tracking retarget now rolls back when the pull
after it fails, is declined or is interrupted; the rest is below, roughly
in the order it wants doing.

1. the integration picker offers a fast-forward that cannot succeed. it
   only appears when the branch has diverged, so "fast-forward only" --
   the default row -- always ends in `fatal: Not possible to
   fast-forward`. replace it. rows wanted: `rebase`, `merge`, `reset to
   <upstream>` (drop the local commits: `reset --keep` onto the upstream
   tip, confirm with default no, undo is `reset --keep <before>` as push
   already does), and `leave as is` (a decline: exit 0, say the branch is
   still ahead n / behind m of its upstream). shared by `gg pull` and
   every switch landing, so one change covers both pastes.
2. the remote switch path mutates before it reads. fetch the picked remote
   and work out ahead / behind first; when the local branch has diverged
   from it, confirm the retarget (name both upstreams and the counts,
   default no). declining stays on the branch with nothing changed, exit
   0. the rollback stays as the net for anything that fails after the
   confirm.
3. a pull prompt cancelled after the checkout leaves you switched (also
   under push follow-ups). record the branch before the checkout and go
   back on cancel, so a cancelled switch ends where it started; mind the
   auto-stash, which must pop on the original branch.
4. name local branches as `local/<branch>` wherever gg prints one, as if
   `local` were a remote: `Switched to branch 'local/main'.`, `Integrate
   'playground/main' into 'local/main' via`, the `gg s` head line, picker
   rows, and the push / pull / delete / branch / empty / checkout-pr
   messages (~9 files by a grep for the branch-quoting formats). the
   picker payload tags (`local:`, `local/remote:`, `remote:`) are parsed
   by parseBranchEntry and double as display text, so split payload from
   display before touching the tags. a remote actually named `local`
   collides with the prefix: add a doctor finding for it (fix: rename the
   remote). `.` as a remote (branch tracking a local branch) wants a look
   at the same time; GetBranchUpstream currently errors on it.
5. a stub-driven test per edge for each of the above, per AGENTS.md:
   every picker row, Esc, failure, and Ctrl-C mid-git.
