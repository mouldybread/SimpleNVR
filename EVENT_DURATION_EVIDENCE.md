# Strict descriptor-bound event video duration evidence

NEW IMPLEMENTATION, base 165986e687ddb3966a2922904c22d9b21fedd006.
165986e green is user-reported, not independently inspected Actions evidence.

## Bounded scope

This adds an opt-in read-only inspection component and regressions, with no
production caller. It is the strict media-duration evidence prerequisite for the
separate requested-duration/full-coverage requirement, NOT that requirement's
completion and NOT a cleanup permit. Existing production revalidation/execution
qualification, publication and unconditional staging retention are unchanged.

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
interruptible. There are no new production probe executions until a separate
reviewed production integration. No registry mutex is used across inspection.

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

## Finite remaining integration

Next compose strict per-input/video timestamp and requested/prebuffer interval
coverage evidence with this component, under an explicit conservative policy;
then bind the complete evidence into production release eligibility/revalidation.
Do not infer full coverage from this duration-only receipt or enable an engine yet.
Positive authorization, same-lease guarded nonrecursive release, explicit partial
receipts/restoration handling, final A01-A12 integration and whole-branch audit/
deployment/browser acceptance remain open. All existing handoff ownership, locking,
metadata budgets, publication ambiguity and retention boundaries still apply.
Main stays untouched; a merge requires separate approval.
