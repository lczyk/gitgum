# meanderings

design notes, proposals, and comparisons. some shipped, some were
rejected, some are still open. **NOT ALL SHOULD SHIP** -- these are just
(sometimes wild) meanderings.

proposals live in `*.prop.md` files. each has a yaml frontmatter:

```yaml
---
status: open | implemented | rejected | shelved
date: YYYY-MM-DD
description: one-line hook
---
```

files prefixed with `_` are no longer "active" (implemented or shelved),
so they sort to the bottom of `ls`.

## style: no verdicts

proposals describe the **gap**, the **options**, **tradeoffs**, and
**open questions** -- not whether to ship. avoid `lean:`,
`recommendation:`, `preference:`, etc. the ship / no-ship call happens
at implementation time with current context; baking a verdict into the
proposal biases that decision and ages badly.

if something has been decided, change `status:` and capture the outcome
in a short closing section (see the `_*.prop.md` files for shape).

## index

run `./index.sh` to print an up-to-date markdown index grouped by
status. other `.md` files in this directory are ignored.
