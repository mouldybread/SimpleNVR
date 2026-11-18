# Expanded correspondence validation and evidence batch

Base: 3f7c2250977518d3f58dc077e5a5bfb75ee81590, green by user report.
This six-file batch supersedes the smaller unpushed assessment-only candidate.
It combines related validation implementation, regressions, evidence and audit
reconciliation without changing production behavior or activating the comparator.

## Stage interruption, preservation and reuse

Eight cases: with/without selected prefix x evidence/comparison stage x explicit
caller cancellation/parent deadline. Owned re-exec children write readiness markers
outside job storage before interruption. The evidence child is a controlled probe
substitute, not real FFprobe cancellation qualification. Both comparison children
must be ready in comparison cases. A concurrent call must be refused as busy while
work is blocked. On return, test-owned evidence work or comparison children are
reaped, readers/comparators/watchers joined, pipes closed, and admission released.
All donor digests, job storage tree, original evidence and lease ownership are
checked. The same admission gate and evidence must then support a successful retry;
terminal release closes inventory and retains staging. The existing shared-deadline
and shorter-parent tests remain unchanged. No production timeout is changed.

## Writer progress and termination

The existing eight native complete-validation rows now include byte/nonempty-file
snapshots before/after each validation observation. Both profiles also require a
writer summary linking measured snapshots, separate bounded forward progress,
explicit stop and joined process state, final bytes/files and lifetime CPU.
A four-second post-measurement wait must witness both byte growth and another
nonempty segment file. This is outside measured validation time and CPU deltas.

An honest progress-during-validation-observation flag may be false: short cached
validation can finish between FFmpeg output flushes. The assessment must not turn
post-measurement progress into a claim of progress during validation. Snapshot
windows include surrounding bookkeeping and between-repeat checks; they are not
an isolated write-latency trace. Counting nonempty files is not counting fsynced
segments. No continuous recorder latency, durability or deployment fit is claimed.
A cancellation exit is expected after explicit stop; the exit diagnostic is kept
rather than misreported as successful process completion. Joining is the invariant.

## All-four-report assessment

Require same-head bound-adapter.json (8), native-prebuffer.json (18),
complete-validation.json (8) and native-track-isolation.json (10), plus both
complete-writer summaries. Keep existing checks; only mark evidence complete after
all pass. Reject missing/duplicate/stale/unauthorized reports and bad lifecycle,
fixture, measurement, refusal-layer, progress or join observations. Preserve input
SHA256 values. Summarize per-stage/combined median/max and observed CPU, separately
reporting profiles with observed byte progress in the validation observation window.
Flattened track error text does not reconstruct Go error identity or caller context;
native Go assertions remain responsible for those stronger invariants.

The existing workflow already discovers the modified Python test filename, runs
all ordinary Go tests with race instrumentation, selects TestCorrResource native
measurements, invokes the assessment even after failure, and uploads its summary
and artifacts. No workflow changes or dependency additions are needed. The expanded
Python regressions retain all earlier cases, add 33 new report faults, 13 writer
faults, positive/negative progress cases, three semantic teardown controls, and
seven end-to-end CLI scenarios. These are synthetic regression inputs, not actual
measurements. CI is the execution gate for this expanded batch.

Historical artifacts lacking the new reports or writer fields intentionally fail
this analyzer; assess old artifacts with the analyzer from their own commit.
The original audit checklist gains a dated current-status section without erasing
historical records or marking unqualified acceptance gates complete.

## Unchanged boundaries

Two cache-warm repeats are descriptive, not tail or intended-host acceptance.
Measurements exclude capture/assembly/publication and include fresh reinspection
of already-published evidence; they are not added production probes. Comparison
child counters do not count FFprobe children. Diagnostic JSON is not a trusted
owner receipt or permission to delete. Resource-cost acceptance, runtime qualification
and cleanup authorization remain false; numerical artifact review and ingestion-time
coverage remain outstanding. main stays untouched; no production source, media
arguments, runtime budget, cleanup policy, merge or deployment changes are included.
