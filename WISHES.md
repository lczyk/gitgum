# lczyk/assert wish list

Notes captured while migrating the fuzzyfinder tests from `t.Errorf`/`t.Fatalf`
to `lczyk/assert`. Each item is something the migration would have used had it
existed; the tests work without these but each one cost a workaround.

## 1. `assert.ErrorIs` (errors.Is semantics)

`assert.Error(t, err, sentinel)` compares via `compare.Errors`, which uses
`Error()`-string equality plus `reflect.TypeOf` equality. That fails for
*wrapped* sentinel errors:

```go
err := fmt.Errorf("wrapping: %w", fuzzyfinder.ErrAbort)
errors.Is(err, fuzzyfinder.ErrAbort) // true
assert.Error(t, err, fuzzyfinder.ErrAbort) // false (different type, different string)
```

The fuzzyfinder tests check ~7 sentinel errors (`ErrAbort`, `context.Canceled`)
that may or may not be wrapped depending on the code path. We worked around
with:

```go
assert.That(t, errors.Is(err, fuzzyfinder.ErrAbort), "expected ErrAbort, got", err)
```

That works but loses the "expected an error" framing. A first-class
`assert.ErrorIs(t testing.TB, err error, target error, args ...any)` (semantics:
`errors.Is(err, target)`) would replace the workaround everywhere `errors.Is` is
used. Probably a one-liner internally.

## 2. `assert.NotNil` / `assert.Nil`

We currently express "value is non-nil" as `assert.That(t, x != nil, ...)`.
Comes up for terminal/finder constructors and channel close detection. Not
critical, but `assert.NotNil(t, x)` reads better and the failure message could
include type info (`"expected non-nil *T, got nil"`).

## 3. `assert.Len(t, slice, n)`

We have a half-dozen `assert.Equal(t, expectedLen, len(slice))` calls. `Len`
would be slightly clearer and could print the slice contents on mismatch
(`"expected len 3, got len 2: [a b]"`), which `Equal` cannot.

## 4. Better `assert.Equal` failure messages for big strings

`assertWithGolden` falls back to `cmp.Diff(expected, actual)` because the
golden fixtures are multi-KB tcell screen dumps where `assert.Equal`'s default
output ("expected X, got Y") is unusable. `assert.EqualLineByLine` exists but
isn't a great fit either — the output is screen content with embedded escape
codes, not line-oriented.

A `assert.EqualWithFormatter(t, want, got, formatter func(want, got T) string)`
would let us plug in `cmp.Diff` and drop the inline `cmp.Diff` call. Niche.

## 5. `b.Loop` warning suppression

Unrelated to assert — `gopls`'s `bloop` linter flags `for i := 0; i < b.N; i++`
in benchmarks, but assert's API is fine. Just noting that the project's
benchmarks (`matching_test.go`, `smith_waterman_test.go`) still use the old
form. Mechanical fix, not blocked by the assert library.
