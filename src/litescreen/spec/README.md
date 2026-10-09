# litescreen models

run `make fizz` from the repo root with the `fizz` cli available. the target
checks every `.fizz` file, prints state counts, and shows the complete
counterexample on failure. generated `out/` trees and `.json` sidecars are
ignored. these are manually maintained abstractions, not proofs of the go
implementation.

## boundary and results

model | boundary | nodes | unique states | result
--- | --- | --- | --- | ---
[lifecycle.fizz](lifecycle.fizz) | terminal ownership, raw-mode failures, signals, suspend/resume and EOF | 5784 | 5054 | passed
[reader.fizz](reader.fizz) | stop, buffered send, poll wake and reader completion | 157 | 148 | passed
[esc.fizz](esc.fizz) | byte arrival, escape timeout and observed channel closure | 413 | 360 | passed
[concurrent_callers.fizz](concurrent_callers.fizz) | two callers, once guards, mutex waits and signal cleanup | 18457 | 17360 | passed
[rendering.fizz](rendering.fizz) | frame cache, terminal cells, cursor, resize and sync | 5150 | 3642 | passed

the lifecycle environment allows two suspend/resume calls, draws, input
bytes and resize events, and one event per signal action, paste toggle and
EOF. the reader has two input and drain events and one stop request. the
escape model consumes at most four bytes. deadlock detection is enabled;
NoOp gives terminal states a successor. crash injection is disabled because
goroutines do not disappear at arbitrary yield points.

the concurrent-caller model lets each of two callers make one call, selected
from Init, Suspend, Resume, Fini, Show, Sync and paste toggling. initial
states cover new, running and suspended screens. one signal and one reader
EOF can interleave. `CallsReturn` assumes successful io and weakly fair
scheduling.

the rendering model chooses one of six scenarios in Init: `diff`,
`controls`, `cursor`, `resize`, `resume` and `external`. each scenario
enables its own environment actions and permits two draws; diff rows range
over all three-column combinations of blank, narrow and wide cells.
`resize` moves and narrows the region; `resume` reacquires the terminal at
a new origin. `external` checks stale-cache repair by Sync. `WRITE_FAILURES`
defaults to false. opt in with `fizz --preinit-hook 'WRITE_FAILURES = True'
...` to check write errors; that run fails on purpose, see the findings.

the reader and escape models check progress under weakly fair scheduling.
the lifecycle model checks safety and reachability only; its IsLive output
does not establish shutdown progress. lifecycle callers are serialised;
reader and signal activity can interleave with their lock sections.

## findings and modelling decisions

- startup must register signals before entering raw mode and hold mu
  throughout initialisation. otherwise Init -> enter raw mode -> unhandled
  signal can exit with a raw terminal. the implementation now has that
  ordering. a signal queued during failed Init must not draw without a grid.
- Suspend and Resume must each hold mu across terminal handover. cleanup
  must see either the handed-back terminal or the fully acquired terminal.
- raw-mode failure coverage records actual failed calls. merely being
  uninitialised or suspended is insufficient: the original assertions still
  passed when both raw-mode calls were forced to succeed.
- signal_seen records a caught signal only. a signal with no registered
  handler terminates the modelled process. SignalThenFini proves cleanup
  after a caught signal whose external handler keeps the process alive;
  ordinary Fini must not satisfy that coverage assertion.
- go select may send when both the send and stop cases are ready. the
  reader model explores both outcomes, fills its one-byte channel on a
  send, and demands immediate exit only when that channel is full. stop
  coverage records the reader position at the stop request.
- escape timeout preserves any outer Alt modifier, including Alt-Esc.
  AltCSIKey observes the emitted modifier; a total byte count cannot prove
  that path. ClosedMidCSI records closure during CSI, rather than a drop
  from an earlier timeout.
- a failed terminal write leaves the frame cache ahead of the terminal.
  the `WRITE_FAILURES` run fails CompletedFrameMatchesBack: a failed Show
  followed by an unchanged Show emits nothing, and the terminal stays stale
  until Sync. accepted as a limitation. os.File.Write already retries short
  writes, EINTR and EAGAIN, so only a hard error such as EIO or a closed
  descriptor reaches that path, and a caller that sees it has lost the
  terminal anyway.
- fizzbee 0.5.4 evaluates transition assertions at `oneof` fork points
  inside atomic actions, with the half-finished state as `after`; `always`
  assertions are unaffected. the rendering model draws the write-failure
  choice before draw mutates anything for that reason. the other models
  either choose first or guard the transition on a counter bumped after
  the choice.

## mutation checks

all 48 mutations below were run in scratch copies and failed with the
intended property. keep mutations outside this directory so generated
sidecars do not become additional inputs to `make fizz`.

model / mutation | property that caught it
--- | ---
lifecycle / unguarded Show | WritesOnlyWhileHeld
lifecycle / signal skips cleanup | ReraiseAfterHandBack
lifecycle / Fini leaks descriptors | FiniReleasesEverything
lifecycle / Resume enters raw mode twice | RawModeNotNested
lifecycle / Init starts two readers | GoroutinesAndChannels
lifecycle / Suspend leaves reader running | SuspendedHoldsNothing
lifecycle / Resume omits reader restart | ReaderWhileRunning
lifecycle / takeover omits paste mode | PasteFollowsRegion
lifecycle / failed Init remains reusable | InitFailureIsSpent
lifecycle / failed Resume closes descriptors | ResumeFailureKeepsSuspended
lifecycle / Init cannot fail | InitRawFails
lifecycle / Resume cannot fail | ResumeRawFails
lifecycle / Suspend disabled | ReachesSuspended
lifecycle / Resume disabled | ResumesWithReader
lifecycle / signals cannot arrive while suspended | SignalWhileSuspended
lifecycle / caught signals always terminate | SignalThenFini
lifecycle / EOF disabled | EOFThenFini
lifecycle / Fini disabled while suspended | FiniWhileSuspended
lifecycle / split startup enters raw mode before signal registration | ReraiseAfterHandBack
reader / lost wake | StopReturns
reader / exit before stop | NoExitBeforeStop
reader / full send ignores stop | SendGivesWayToStop
reader / stop excluded while parked | StopWhileParked
reader / stop excluded while sending | StopWhileBlockedSending
reader / stop excluded while checking | StopWhileChecking
escape / timer has no fairness | ReturnsToIdle
escape / emitted bytes unaccounted | EveryByteAccounted
escape / close leaves parser open | NothingAfterClose
escape / timeout emits wrong key | TimerAfterEscKeepsModifier
escape / cut CSI emits a key | CutCSIDropped
escape / bytes excluded after timeout | SplitSequenceGivesTwoEvents
escape / closure excluded during CSI | ClosedMidCSI
escape / Alt modifier omitted from CSI event | AltCSIKey
concurrent / signal cleanup skips hand-back | SignalRestoresTerminal
concurrent / input closed despite reader EOF | ChannelsAndReadersSafe
concurrent / second Fini returns without waiting | FiniReturnsReleased
concurrent / Resume ignores the suspended and done guards | ChannelsAndReadersSafe
concurrent / Show ignores done | WritesWhileOwned
concurrent / reader started after EOF | ChannelsAndReadersSafe
concurrent / Suspend leaves the reader running | SuspendedTerminalReleased
concurrent / callers have no fairness | CallsReturn
rendering / every Show writes | UnchangedShowIsSilent
rendering / control runes emitted as is | CompletedFrameMatchesBack
rendering / draws ignore suspended | SuspendedCallsAreSilent
rendering / right-edge wide rune not clipped | CompletedFrameMatchesBack
rendering / overwritten wide rune keeps its tail cached | CompletedFrameMatchesBack
rendering / Resume keeps the cache | CompletedFrameMatchesBack
rendering / Sync keeps external damage | SyncRepairsDamage
rendering / WRITE_FAILURES on | CompletedFrameMatchesBack

the startup mutant splits the lock section to expose the unregistered
window. the implementation test also checks registration inside enterRaw;
atomicity in the model alone cannot establish the ordering of go calls.

## implementation tests

requirements | tests
--- | ---
lifecycle R1-R4, R7-R8 | `TestScreen_SignalRegisteredBeforeRawMode`, `TestScreen_InitRawFailureWithdrawsSignal`, `TestScreen_SignalRestoresThenReraises`, `TestScreen_FiniBeforeInitIsInert`, `TestScreen_InitTwiceIsRejected`
lifecycle R5-R6, R9-R10 | `TestScreen_SuspendPausesInput`, `TestScreen_SuspendLifecycle`, `TestScreen_SuspendTogglesPasteMode`, `TestScreen_SignalInsideSuspendReraisesRestored`, `TestScreen_SignalInsideResumeReraisesRestored`
lifecycle R8 retry | `TestScreen_ResumeRawFailureKeepsSuspendedAndAllowsRetry`
concurrent callers | `TestScreen_Fini_Idempotent`, `TestScreen_InitTwiceIsRejected`, `TestScreen_InitAfterFiniIsRejected`, `TestScreen_SuspendLifecycle`, `TestScreen_FiniEndsSignalLoopQuietly`
rendering R1-R6 | `TestFramebuf_FlushMatchesTerminalModel`, `TestRegressionFlushSanitisesControlRunes`, `TestRegressionWideRuneLastColumnBlanked`, `TestRegressionWideRuneShiftedRightReemitted`, `TestRegressionAdjacentWideRunesShiftedLeftReemitted`
reader shutdown | `TestScreen_Fini_StopsReadLoop`, `TestScreen_FiniWakesBlockedReadLoop`, `TestFdInput_WaitReturnsOnWake`, `TestFdInput_WakeOutsideSetupIsSafe`
escape events and timeout | `TestParseEscape_BareEsc`, `TestParseEscape_Alt`, `TestParseCSI_Malformed`, `TestParseEscape_CSIArrow`

run `go test -race ./src/litescreen/...` for these implementation checks.
the startup ordering test was also run against the pre-fix implementation
using a go overlay: it failed before the fix and passed against current code.

## limits

- arbitrary concurrent lifecycle callers, terminal write failures and
  failed raw-mode restoration are excluded from the lifecycle model.
- the concurrent-caller model covers two calls total, one per caller; it
  does not prove safety for unbounded callers or every API call sequence.
- rendering output failures are excluded from the default run. the
  optional `WRITE_FAILURES` run explores zero-byte failures separately and
  fails by design.
- the lifecycle model assumes reader shutdown finishes. the reader model
  checks that handshake after input setup, with nonblocking reads and no
  EOF or input errors; it deliberately excludes the poll timeout fallback.
- signals queued during failed Init are checked by implementation guards,
  not represented explicitly in the lifecycle model. SIGHUP and uncatchable
  signals are excluded.
- cursor queries, type-ahead ordering,
  actual key mappings, UTF-8 and SS3 are excluded. CSI completion abstracts
  the key mapping; it does not establish which byte strings are valid keys.
- the escape model's Close means observing a drained, closed channel.
  it does not forbid draining bytes buffered before channel closure.
