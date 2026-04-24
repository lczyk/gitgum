taste-scoping-version: 1

- Return errors as values, always wrap with `fmt.Errorf("context: %w", err)`. Never panic.
- Prefer longer functions with section-header comments over splitting into many small functions. Comments delimit logical sections, not every step.
- Write comments for non-obvious invariants, subtle state, or tricky conditions. Enforce at compile-time if possible; cheap runtime check + comment if not; heavy test coverage as a last resort.
- Comments written in casual style -- matching how i'd write them, colloquialisms and all. no forced capitalisation. sentences don't need to start with a capital. acronyms lowercase too (http, url, grpc) -- only capitalise if it looks genuinely weird lowercase.
- Extract helpers only if genuinely called from multiple places, or if the abstraction makes things more testable / encapsulates a meaningful higher-level concept. Don't split just to hit low LOC counts.
- Use interfaces + DI to make things testable. Prefer DI over mocks.
- Unit tests first. Example-based unless it takes excessive code yoga. Table-driven where it fits naturally.
- Delete dead code, compat shims, and legacy fallbacks freely. Personal project, no external users.
- Truthful names. Find abstractions that keep names short -- complex things earn abstract names (e.g. `State`), detail lives in the code.
- Compact public API. Internals can sprawl. 2k line files fine if cohesive.
- Split modules at responsibility boundaries: things that change for different reasons, have different callers, or require different mental models to understand each side.
- No strong opinion on when to introduce or collapse interfaces -- use judgment case by case.
- Extract cross-cutting concerns (retry, logging helpers, etc.) to a shared package by default. Inline duplication only with a specific reason.
- Prefer smaller-scoped refactors (easier to track), but wide-blast-radius changes are fine when the situation calls for it. No feature flags or staged rollouts needed.
