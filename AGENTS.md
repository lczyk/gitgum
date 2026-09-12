# agents.md

- this file represents long-term project memory.
- this file gets edited ONLY by humans, NEVER by agents.
- direct statements addressing the reader are addressed to the agent reading this file -- to you.
- qualifiers like "NEVER", "ALWAYS", "SHOULD", "MUST", "MAY" are used to concretely pin the desired outcomes.
- "IFF" means "if, and only if"
- if there are observations or rules you think ought to be saved to long-term project memory, and if you are working in an interactive session, you MAY prompt the user with that suggestion. NEVER make edits yourself.

---

## general rules

- `VERSION` file at the repo root is the source of truth for the version; everything else that records one ought to be kept in sync with it.
- do not bump versions or make release commits. leave `VERSION` and the manifests at the released number. the releases are performed by the user.

## interactive commands design

- minimise network interactions. whenever possible batch them together and only fetch the minimum information to perform user interaction.
- whenever possible favour responsive design where a user is shown a partial result with slow, network-bound interactions happening concurrently.
- don't assume that the network interactions are fast, or that there is network connection at all. try to maximise functionality without a network connection.
- MUST be designed against the command's full user-journey graph, not just the branch being edited. trace it whenever a
  prompt or a mutation is added or changed.
  - edges: every answer to every prompt (each row, plus Esc / Ctrl-C), every failure, and Ctrl-C while a git child is
  running.
  - every leaf MUST be one of: the goal reached; a decline (exit 0, SHOULD say what was left undone); a cancel
  (exit 130); a reported failure (exit 1).
  - a decline MUST route onward when there is somewhere else to go -- e.g. declining one push destination offers the
  others.
  - there SHOULD be a stub-driven test for each edge.
- each interactive command SHOULD be as transactional and rollback-able as possible. each user interaction SHOULD end with either the world state mutated as the user desired, or no overall change to the to the user state at all. facilitate this, use tehse design principles:
  - begin each user interaction in a read-only mode and try to ask questions of the user untill you cannot proceed without mutating some world state. only then do the minimal required mutations before the next user interaction.
  - whenever possible use backups states such that, if you are required to rollback an operation, due to later user cancel or otherwise a controlled abort, you are able to restore the initial state of the repo.
  - NEVER clean up backups before the user interaction is over. ALWAYS clean up backups AFTER it is over, regardless of the outcome.
  - pending changes SHOULD happen in order of how easily they can be undone, and the record of what has been applied and how it ought to be undone must be kept.
  - NEVER assume a failed outward-facing step had no effect: check (e.g. re-fetch after a failed push) before rolling back around it.
  - Ctrl-C while a git command runs triggers rollback
