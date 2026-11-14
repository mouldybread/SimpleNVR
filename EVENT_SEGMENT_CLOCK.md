# Segment-level NVR notification timing

## Scope and withdrawn proposal

The user selected the simpler segment-level approach. Per-packet/frame hashing,
framehash companion outputs and the previous uncommitted packet collector are
withdrawn. None of those draft files belongs in this change set. No custom FFmpeg,
additional process/connection, runtime media argument, probe, descriptor, lease,
worker or checkpoint schema is introduced here.

EVENT_NVR_INGESTION_POLICY.md still governs the clock and conservative retention.
This batch adds coarse production notification observations, NOT a complete
implementation of requested-interval ingestion coverage.

## What is sampled

The existing prebuffer FFmpeg process already writes completed-segment CSV rows.
Immediately after csv.Reader.Read returns, the existing reader samples time.Now.
One small record stores the session nonce, row sequence, process Start-return time,
previous/current receipt samples and that row's declared media start/end.
Invalid rows or rejected file registration mark later observations uncertain.
Backward wall/monotonic ordering is flagged. Forward jumps/drift/UTC accuracy are
NOT qualified; no threshold or invented uncertainty allowance is supplied.

The sample is COMPLETION-NOTIFICATION RECEIPT, not media/socket arrival. It includes
input/probing/mux buffering, CSV/pipe buffering and reader scheduling delay. Do not
subtract duration from receipt and call the result an established ingestion start.
CSV boundaries are declarations, and gaps/overlaps in them are diagnostics rather
than proven missing/duplicated camera footage. Session changes are not interpolated.

## Small lifetime and existing ownership

Metadata has one entry per currently indexed segment. The existing bufferMutex
protects the metadata and buffer together. Successful index pruning drops its
metadata; failed pruning retains both. Normal buffer teardown drops the metadata
with the index and still preserves existing recovery bytes.

During existing real pinning, metadata is copied by value with the frozen ordered
inputs. Source path/declarations and device/inode/size/mtime must match the actual
pinned entry, otherwise timing is missing. Link creation can change ctime, so these
coarse origin checks do not replace the existing full post-pinning inventory checks
or constitute a content-integrity proof. Duplicate indexed source names drop timing
association. No descriptor is opened or adopted to obtain timing.

The event worker logs a small summary of missing/uncertain records, declared gaps,
overlaps, sequence breaks, session changes and backward clock observations. It does
not acquire a media lease or inspect the mutable buffer to generate that summary.
There is no packet ledger, extra media output, frame decoding or hashing.

## Unchanged behavior and limits

Selection rules, recording PTS/DTS, B-frame/A/V handling, filenames, stream-copy MP4,
faststart, publication/partial publication, persistence failures, checkpoints,
probe counts/budgets, leases, descriptor teardown, slot return and child-process
interrupt/grace/kill/join remain unchanged. Event staging remains unconditionally
retained. A zero-warning summary is NOT positive coverage or a cleanup capability.
The existing single event-capture file keeps its already-shipped Run bookends;
this patch does not reimplement them or relabel them as media bounds.

Observations are in memory, not durable recovery metadata. Restarted/legacy clips
have no certified observations and are not retroactively certified. Requested-start
selection and UI timestamps are not changed to estimated ingestion times by this
batch. No new administrative API, recovery sweep, merge or deployment is included.

## Tests and finite continuation

The new Go tests cover clock ordering/uncertainty, declared gap/overlap/session
classification, origin substitution, frozen production selection, index lifetime,
duplicate ambiguity, retention/slot teardown and the actual CSV read/cancel/join
loop with a labeled synthetic child. They run through existing package discovery;
no workflow changes. Application acceptance requires exact-head GitHub Actions,
including core/race/vet/storage/lifecycle and BOTH isolated Chrome runs.

Next: use these coarse records and existing event-capture evidence to represent
requested-interval coverage and remaining unknown boundaries without adding packet
tracking. If coarse notification timing cannot prove a boundary, preserve the
unknown and retain staging; do not claim exact arrival. Output correspondence and
positive eligibility require their own review before any release-engine activation.
Then finish partial outcomes/ownership acceptance and remaining whole-project A-D.
