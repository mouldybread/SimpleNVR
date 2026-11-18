# Strict event video continuity arithmetic — NEW IMPLEMENTATION

Base: 088ab01670a825dd7c7ef5afba44197556825afe.
Base green is user-reported. No independent Actions result was retrieved.

## Scope

This batch adds an opt-in PURE validator and regression tests. It has no production
callers and does not change the existing duration collector or revalidation.
It does not add FFprobe invocations, lease acquisitions, descriptors, goroutines,
checkpoints, schemas, deletion or cleanup authorization. Main stays untouched.

Each explicitly observed presentation interval must have a positive duration and
its start must exactly equal the preceding end. One-tick gaps, overlaps, duplicate
starts, reordered observations and overflowing endpoints all refuse. No sorting,
interpolation, format-duration fallback, rounding, epsilon or percentage allowance
is used. Variable positive frame durations are permitted when endpoints abut.
Zero PTS is valid; negative and nonzero origins are not normalized away.

The caller must provide presentation-order VIDEO frame intervals in one positive
rational time base. Demux packet/decode order must NOT silently be used as
presentation order. This validator neither reads nor authenticates observations.
The resource ceiling is 1,048,576 intervals; exceeding it refuses rather than
truncating. Entry, every interval and completion check cancellation. The input
slice is not modified or retained. Arithmetic outputs contain only scalar values.
Length comparison uses exact big integers for subtraction and multiplication,
including spans crossing the int64 subtraction boundary. It compares LENGTH only;
a shifted origin cannot establish a requested absolute time interval.

## Tests and validation

Tests cover exact/just-short durations, rational bases, signed timestamp origins,
endpoint limits, overflowing products/subtraction, every single-tick boundary
perturbation over 2–64 frames, invalid/empty input, caller immutability, resource
refusal, and deterministic entry/mid-loop/completion cancellation. All observations
are explicitly synthetic. No real-camera, decoded-frame or browser acceptance is
claimed. This new standalone arithmetic component is not full-branch validation.
Existing Go package discovery should collect these tests after an approved push;
no workflow changes are included. Both Chrome runs and aggregate checks remain.

## Still required before any positive eligibility

1. A bounded, strict descriptor-bound reader for COMPLETE video presentation
   observations with explicit timestamp/duration presence, EOF/process success,
   stream identity and refusal of truncated/missing/ambiguous data. Do not infer
   a last frame's duration or treat container duration as frame evidence.
2. Per-input and assembled-output continuity/ordering checks with the exact owned
   inventory, execution, job/request and live manager bindings before/after I/O.
   No new media processes are authorized by this arithmetic-only batch.
3. Explicit requested wall-clock and selected-prebuffer interval provenance;
   event StartTime plus a duration is not sufficient to establish that mapping.
4. A conservative, separately reviewed composition policy for assembled coverage,
   and all remaining positive-authorization, partial-receipt and release checks.

The existing up-to-two duration probes and their aggregate budget are unchanged.
Preserve unconditional staging retention, publication/failure behavior, independent
metadata budgets, descriptor ownership, terminal lease handoff and slot return.
This component cannot mint an observed receipt or activate a cleanup engine.
