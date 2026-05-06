# `make bench` linux wedge -- diagnosis (corrected) and fix

## symptom

`make bench` on linux (qcom-x1e dell xps13-9345, arm64) hard-wedges the
machine -- forced reboot needed. same bench passes on macos.

## evidence

`journalctl -k -b -1` (previous-boot kernel log) ends abruptly mid-stream
with normal ufw block lines, ~1 min before next boot. **no oom-killer
banner, no panic stack, no rcu stall, no shutdown messages.** kernel
went silent and was hard-reset.

that pattern rules out a clean oom-kill (which is loud, multi-screen
stacktrace before acting) and points to a true wedge -- scheduler /
memory pressure so severe the kernel couldn't drive the printk path
before something hung the world.

## what was originally hypothesised, partially corrected

the first-pass theory blamed `bench/normal`: claimed the
`go bs.ChannelEvents(eventsChan, nil)` goroutine in `NewWithBenchScreen`
fires before `SetEvents` runs, drains an empty `s.events`, exits, and
leaves the finder blocked on `termEventsChan` forever -- so
`bench/normal` would deadlock on iter 1 of `b.Loop()`.

experimentally, the **iter-1 deterministic deadlock** part is wrong --
`bench/normal` runs 6498 iters cleanly at `-benchtime 1s`. but the
underlying race mechanism (one-shot `ChannelEvents` drain vs `SetEvents`
staging) is real. it's just **probabilistic**, not deterministic: the
test goroutine usually wins the lock first because go's scheduler
doesn't preempt the current goroutine to run a freshly-`go`'d one, but
"usually" isn't "always". over thousands of iterations and varying
scheduling pressure, the race is eventually lost.

## actual root cause (verified)

two independent bugs combine. neither is in `bench/normal`.

### 1. `bench/hotreload` silently wedges

with the broken code, isolating the sub-benchmark:

```
$ timeout 60 go test -run '^$' -bench 'BenchmarkFind/bench/hotreload$' -benchtime 1s -timeout 30s ./src/fuzzyfinder/
exit=124  (no output at all)
```

zero output, even the `-timeout 30s` panic + goroutine dump doesn't
fire -- the process is too far gone to respond. only the outer wall
timeout kills it. **this part is reproduced and verified.**

`bench/hotreload` differs from `bench/normal` only in passing a
`&sync.Mutex{}` instead of `nil` to `f.Find`. **goroutine-dump evidence
confirms the deadlock is the broken event-relay race**: triggering a
`SIGABRT` after 8s of wedge prints 22 goroutines total, including:

- `goroutine 7`: parked at `fuzzyfinder.go:616` = `<-f.termEventsChan`
  inside `readKey`. the bench iteration is waiting for an event.
- `goroutine 194, 195`: the finder's own bg ticker + event-handler;
  fine, both parked in normal `select` waits.
- **no `BenchScreen.ChannelEvents` goroutine.** the spawned relay
  already ran, drained nothing (lost the race vs `SetEvents`), and
  exited. nothing else pushes into `termEventsChan`, so `readKey` is
  stuck forever.

why this happens in `bench/hotreload` but not `bench/normal` (same race,
same iteration scale) is _not_ confirmed. the hot-reload locker likely
shifts scheduling in a way that makes the spawned goroutine more likely
to win the lock-race, but the exact mechanism wasn't traced. what _is_
confirmed is that the deadlock is event-relay starvation -- not some
new locker-induced bug.

### 2. `sim/*` leaks one goroutine per iteration

`helper_test.go:17` spawns `tcell.SimulationScreen.ChannelEvents` with
`nil` quit chan. that's a long-running relay; it parks forever waiting
on the never-closing quit. one stuck goroutine per `NewWithMockedTerminal`
call.

verified directly:

```
goroutines before: 2
goroutines after 200 iters: 202 (delta=200)
```

precisely 1:1. at `-benchtime 1s` that's ~1100 leaks in `sim/normal`
and ~860 in `sim/hotreload` -- ~2000 stuck goroutines accumulated
before `bench/hotreload` even starts.

### why this wedges linux specifically (speculative)

**none of this section is verified against the original kernel-wedge
symptom -- the wedge has not been reproduced under controlled
conditions.** what follows is a plausible story that fits the verified
ingredients (bench/hotreload deadlock + 1:1 sim leak), not a confirmed
causal chain.

candidate story: `bench/hotreload` deadlocks while ~2000 leaked
goroutines from `sim/*` sit pinned in the runtime, with all their
associated finder / sim-screen state holding heap. the go runtime is
gc-churning over many pinned objects while the scheduler tries to find
anything runnable. macos vm/scheduler handles this differently and
either keeps churning or oom-kills the test process cleanly. linux on
this arm64 box wedges so hard the kernel can't even log -- speculated
to be cpu cores spinning in the go scheduler at high priority while
swap thrashes.

caveats:
- the macos-vs-linux scheduler claim is carryover from the original
  theory; no new evidence supports it
- the "~2000 goroutines + a deadlock" load has not been benchmarked
  against the actual kernel-wedge threshold
- prior to commit `a73c709` `BenchmarkFind` was a single unnamed loop;
  whether the sim leak alone (without the bench/hotreload deadlock)
  ever wedged the box was not measured

## fix (applied in `5a38642`)

minimal patch in `src/fuzzyfinder/bench_screen_test.go`:

1. drop the `go bs.ChannelEvents(eventsChan, nil)` call in
   `NewWithBenchScreen`
2. give `BenchScreen` direct access to the finder's event chan
   (`eventsChan chan<- tcell.Event` field)
3. `SetEvents` pushes events into that chan in a goroutine (async so
   staging > buffer-size events doesn't deadlock against a not-yet-reading
   finder)
4. `ChannelEvents` becomes a quit-blocking noop -- only kept to satisfy
   the `screen` interface; never called on the bench path

verification with fix applied -- all four sub-benchmarks complete:

```
BenchmarkFind/sim/normal-12        597   1586980 ns/op   937674 B/op   15366 allocs/op
BenchmarkFind/bench/normal-12     3436    217248 ns/op    58130 B/op     139 allocs/op
BenchmarkFind/sim/hotreload-12     378   1531235 ns/op   937362 B/op   15366 allocs/op
BenchmarkFind/bench/hotreload-12  2685    243325 ns/op    58136 B/op     140 allocs/op
PASS
```

allocs match the ~136 figure in the original commit message. full test
suite still passes.

## what's still leaky (not fixed by `5a38642`)

the `sim/*` goroutine leak in `helper_test.go:17` is independent of the
bench-screen bug and is **not** addressed by the fix. with the fix
applied, `make bench` completes -- but each invocation still leaks
~2000 goroutines for the duration of the test process. the process
exits cleanly so the leak doesn't persist, but a long-lived test
runner / repeated invocations would still pile up state.

worth a follow-up: pass a quit chan to `ChannelEvents` in
`NewWithMockedTerminal` and close it via `t.Cleanup`. without the
`bench/hotreload` deadlock pinning the leaked state indefinitely, the
linux wedge no longer triggers -- so this is hygiene, not a blocker.

## defence-in-depth (optional)

add a `context.WithTimeout(ctx, 30*time.Second)` around each `f.Find`
call in `BenchmarkFind` so any future regression of this shape returns
via `ctx.Done` rather than wedging. the silent-wedge property of
`bench/hotreload` -- where even `-timeout` couldn't dump goroutines --
is the worst-case symptom; an in-bench ctx timeout would surface it as
a normal error.

## investigation log

experiments run, in order, against the broken pre-`5a38642` code:

1. **`bench/normal` at small N** -- `-benchtime 5x`, `-benchtime 100x`,
   `-benchtime 1s` (~6500 iters). all completed cleanly. disproved the
   original "iter-1 deterministic deadlock" claim.
2. **full `BenchmarkFind` matrix at `make bench` settings** -- 3 of 4
   sub-benches completed; `bench/hotreload` did not produce output and
   the inner `-timeout 30s` did not fire. localised the deadlock to
   `bench/hotreload` specifically.
3. **isolated `bench/hotreload`** -- silent wedge confirmed; `-timeout`
   ineffective.
4. **goroutine leak counter for `sim/*`** -- standalone test running
   `NewWithMockedTerminal` + `Find` in a loop with
   `runtime.NumGoroutine()` deltas. result: exactly 1:1, 200 iters ->
   delta=200.
5. **SIGABRT goroutine dump on the wedged `bench/hotreload`** --
   compiled the test binary with `go test -c`, ran with a watchdog
   shell that sends `SIGABRT` after 8s if the process is still alive.
   produced 22-goroutine dump showing the bench iter parked at
   `fuzzyfinder.go:616` (`<-f.termEventsChan`) and no `ChannelEvents`
   relay goroutine. confirmed event-relay starvation as the deadlock
   mechanism.

key methodology observation: when `go test -timeout` doesn't fire on a
wedged process, an external `SIGABRT` watchdog is the cheapest way to
get goroutine stacks. `kill -ABRT <pid>` causes the go runtime to dump
all stacks to stderr before aborting; no special build flags needed.

key diagnostic observation: directional-right + specifics-wrong is a
common failure mode for first-pass theories. the original theory
correctly identified the broken `ChannelEvents` one-shot drain as the
root cause but mis-located the symptom (`bench/normal` instead of
`bench/hotreload`) and over-claimed determinism (iter-1 instead of
probabilistic). the fix is correct either way -- but the diagnosis
needed verification before being trusted as a complete explanation.
