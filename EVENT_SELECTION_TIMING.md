# Production request/selection timing provenance — NEW IMPLEMENTATION

Base: 4028ef5fad2c3c67049f76784e1bb67bde240b2a. Remote audit head verified.
Base and preceding db53aa8 green are user-reported, not independent Actions evidence.

## Audit finding: no trustworthy requested-interval mapping yet

The HTTP handler accepts client start_time independently of capture scheduling.
Pinning selects completed buffer entries ending after host now minus bufferDuration,
not entries selected against the requested start_time. Segment StartTime is parsed
from a second-resolution filename. EndTime adds a float CSV-derived duration to
that filename time. The CSV begin value is subtracted to derive duration, but its
absolute relation to filename/client wall clock is not established or retained.
The worker later starts a separate RTSP capture for the requested duration.

Thus these values, plus successful FFprobe duration/frame continuity, cannot prove
that the output spans the requested wall-clock interval. This batch does not change
HTTP semantics, choose a tolerance, infer frame times, or relabel declarations as
trusted media anchors. Missing/invalid declared times are preserved as declarations,
not silently repaired or interpreted as coverage. No positive coverage API is added.

## Actual production integration

After successful real pinning and its pinned checkpoint, the execution observation
now freezes request ID/duration/StartTime/CreatedAt, preStart, selection instant,
cutoff, pin-completion instant and exact ordered selected-input records. Each record
copies the source path and declared times and binds its staged path to the already
owned descriptor/recorded inode identity. The existing buffer read lock and setup
lease are still held. No new lease or descriptor is acquired, and the live buffer
slice is not retained, reread or used to refresh identities after pinning.

An identity-bound self reference makes copying the selection observation an invalid
substitution. Execution binding now requires the same frozen request, same ordered
preFiles and same inventory pins. StartTime or selection changes before capture,
assembly or later revalidation refuse execution qualification. This closes a
mutable-plan gap: the older execution observation froze Duration but not StartTime,
CreatedAt or the ordered pinned-input plan. Normal ring pruning does not change
the frozen record and is not re-adopted as new provenance.

The owned capture path records host times immediately before and after the ACTUAL
cmd.Run call, including failed/canceled returns. Capture qualification additionally
requires a successful ordered Run observation plus the already-existing creator
registration/close and genuinely persisted captured checkpoint. Run timing alone
cannot qualify capture/publication. Replay, missing entry, reversed bookends or
changed bindings refuse. The old manually constructed no-inventory path remains
unobserved and cannot mint production provenance.

These are host RUN-CALL bookends, including startup, RTSP connection and draining;
they are NOT first/last-frame timestamps, not a media-PTS anchor and not a guarantee
that all intervening seconds were recorded. time.Now observations retain Go's local
monotonic component where present; no global clock hook, sleeps or local context is
stored. Normal deferred assembly/capture context cancellation does not erase a
successful observation; actual live-manager cancellation still refuses execution.

## Unchanged boundaries

Selection policy, media command arguments, process count, FFprobe limits, shared
inspection budget, capture/assembly returns, publication and checkpoint semantics
are unchanged. Observational refusal does not roll back publication or skip
independent retained metadata/descriptor/slot/lease teardown. Terminal staging
retention remains unconditional. No cleanup permit, deletion, extra media process,
global registry map, goroutine, persistent descriptor or checkpoint schema is added.
No main write, merge, deployment, workflow or HTTP behavior change is included.

## Validation scope and next step

Regressions exercise actual production pinning, source-index mutation without
snapshot mutation, exact pin ordering/identity, shifted request/creation/origin
times, copied/missing observations, capture Run sequence/failure/cancellation,
normal deferred context cancellation, production revalidation and terminal lifetime.
Synthetic Run bookends explicitly do NOT certify actual footage. Declared malformed
segment times remain visibly untrusted. Tests use deterministic supplied instants,
not timing sleeps. Existing Go package discovery includes the new test file.

Preparation verifies modified base Git blobs, patch application and unchanged media
command lines/process sites. Go/gofmt are unavailable locally: compilation, Go tests,
vet, race, both isolated Chrome runs and full-branch checks have NOT run. Earlier
reported green applies to the BASE, not to this candidate. New-head validation is
required after a separately approved push.

Next establish a trustworthy producer/media-to-wall-clock mapping or conservative
provable bounds, then compose selected-prebuffer/capture and assembled coverage.
Changing media-process behavior requires separate explicit review; do not add
per-input probe processes under this provenance-only batch. Partial-release receipts,
positive cleanup authorization, remaining A01-A12 integration and deployment/browser
acceptance remain open. Ambiguous interval mapping must remain ineligible for cleanup.
