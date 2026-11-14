# Strict descriptor-bound event video duration evidence

NEW IMPLEMENTATION, base 165986e687ddb3966a2922904c22d9b21fedd006.
165986e green is user-reported, not independently inspected Actions evidence.

## Bounded scope

The reader introduced in 597f587 is now wired into production finalization and
read-only release revalidation. It remains only the strict media-duration lower
bound prerequisite for requested-duration/full-coverage validation, NOT that
requirement's completion and NOT a cleanup permit. Publication return behavior,
execution qualification and unconditional staging retention are unchanged.

The collector requires a production execution-qualified finalizing job and its
already transferred terminal media lease, inventoried capture descriptor and
publication-derived output descriptor. It does not discover/open media by name,
acquire/release a lease, write a checkpoint, alter recoveryFailed, or add pins.
Full inventory preflight and published-output checks run before inspection and
again afterward, with job/manager/inventory/borrow/output/execution and request
bindings. The receipt becomes unusable after identity change or terminal teardown.
No local metadata/probe context is retained in the receipt. In particular, an
already-normally-canceled assembly-local context is not revived or made live again.

The production probe function uses inherited child fd 3 at /proc/self/fd/3, MOV
input and file/pipe protocol whitelist, v:0 selection, and bounded JSON output.
One aggregate five-second inspection context covers both probes and checks;
WaitDelay bounds child pipe teardown separately. The caller owns the held shared
lease and serializes this component with job teardown. Kernel I/O is not made
interruptible. For an execution-qualified successful publication/checkpoint,
production now runs up to TWO additional read-only FFprobe processes under that
SAME held finalization lease. Capture/concat processes remain outside ownership.
This extends the existing publication-probe exception; inspection can delay other
media mutations up to its bounded deadline/child pipe teardown, plus uninterruptible
kernel I/O if storage blocks. No registry mutex is used across inspection.

## Exact lower bounds, not invented coverage policy

Only video-stream duration_ts and time_base qualify. There is no container/format
duration fallback, decimal-seconds rounding, slack, percentage or tolerance.
Positive int64 ticks and rational time-base components are compared using exact
bounded big-integer products, avoiding overflow. Capture AND published output must
independently reach the integer requested event duration. A long prebuffer/output
cannot compensate for a short capture. Missing, invalid, zero/negative, overflowing,
ambiguous or short evidence refuses; failed/canceled probes yield no receipt.

This is a metadata lower bound, not proof that every frame exists or decodes, that
video has no timestamp gaps, or that footage covers the requested wall-clock range.
In particular start_time can be supplied independently of current capture and
prebuffer selection: a duration cannot establish that time mapping. The existing
export/timeline duration probe permits a format fallback for its separate purpose;
that API/cache is deliberately neither weakened nor reused as cleanup evidence.

## Test/validation scope

Tests cover exact and just-short ticks, rational bases, overflow-safe products,
invalid/missing/video-versus-container evidence, process/parser/cancel refusal,
held-fd use after pathname replacement, actual MP4 video-stream tick probing,
separate capture/output insufficiency, changed inventory/output refusal before
probe, bound job/request/lease/manager lifetime, read-only failure semantics,
normal deferred context cancellation and retained terminal slot/lease teardown.
Synthetic witnesses are explicitly not camera/coverage proof. The real MP4 test
checks FFprobe's duration fields on generated media, not full event acceptance.

Existing GitHub Actions package-test discovery covers these regressions. Core,
storage/lifecycle, both isolated Chrome runs and aggregate required checks stay
unchanged. No workflow changes are needed or included. Exact new-head Actions
validation remains required after approved push.

## Production observation and remaining integration

After successful durable publication and the published checkpoint, execution is
snapshotted and acquisition ownership transfers to the job. Duration collection
then runs once with its existing aggregate deadline rooted in the live manager,
not the publication/retained-checkpoint metadata context. The SAME lease and pins
survive to terminal release. Duration failures yield no receipt, do not roll back
publication, do not set/clear recoveryFailed, and do not prevent independent
retained persistence or descriptor/slot/lease teardown. Ineligible execution skips
probes entirely. A failed duration observation is not automatically retried.

Revalidation binds the exact duration receipt, request, capture/output descriptors
and execution/job/inventory/manager/borrow identities before and after its existing
filesystem checks. Missing/failed/substituted evidence refuses. Terminal checks
reuse the receipt and revalidate identities; they do not run more media processes.
The earlier synthetic revalidation fixture now explicitly emits stream-duration
JSON through an actual probe executable; no duration/execution bits are assigned.

Next compose strict per-input/video timestamp and requested/prebuffer interval
coverage evidence, under an explicit conservative policy, and bind that complete
evidence into positive eligibility. Duration integration alone does not do so.
Do not infer full coverage from this duration-only receipt or enable an engine yet.
Positive authorization, same-lease guarded nonrecursive release, explicit partial
receipts/restoration handling, final A01-A12 integration and whole-branch audit/
deployment/browser acceptance remain open. All existing handoff ownership, locking,
metadata budgets, publication ambiguity and retention boundaries still apply.
Main stays untouched; a merge requires separate approval.
