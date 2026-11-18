# Event failure evidence preservation

## Preservation decisions

A later prebuffer hard-link failure no longer recursively removes earlier successful pins. Admission still fails and releases its slot without queuing capture or reporting event acceptance. Earlier pins remain in their unique event staging directory even if buffer pruning subsequently unlinks their source names. If the first pin fails, only an empty-directory removal is attempted; unexpected contents or removal failures are left in place. Retained partial pinning currently has no durable event identity manifest.

The defensive queue-refusal path retains pinned inputs and releases its slot. Cancellation before capture continues to preserve pinned inputs while allowing empty, never-started queued staging to be removed.

Once capture is attempted, a process error or cancellation retains its staging directory regardless of whether an output stat would show nonempty media. This deliberately permits empty failed-capture directories to remain: a failed or unavailable size measurement must not authorize recursive evidence deletion. Successful capture does not independently certify the requested duration or completeness.

Assembly begins with staging preservation enabled. FFmpeg errors or assembly-context cancellation select a partial output name. Existing bounded publication may still publish a readable partial, but successful partial publication no longer authorizes deletion of original capture, pinned prebuffer or concat list. Publication/validation failures also preserve staging and existing nonempty root temporary outputs. Cancellation observed during publication conservatively preserves staging too.

Only successful assembly and publication without observed cancellation retain the existing staging-removal behavior. The validator checks a video stream and readable packet, not every frame, complete coverage or requested duration. This batch does not strengthen that validator, promise power-loss durability, or make all successful media lossless evidence substitutes.

## Storage and recovery limits

Retained staging consumes logical space and pinned disk blocks; repeated failures, including empty failed captures, can accumulate directories. Existing opt-in admission thresholds are observations, not reservations, runtime quotas or write-failure watchdogs. No evidence is deleted to relieve disk pressure. There is no new automatic reassembly, expiry, retry, operator action endpoint or durable recovery manifest.

Stop cleanly and preserve separate copies of configuration, metadata and affected camera directories before manual recovery. Do not execute retained concat lists blindly. Published partial MP4s remain subject to ordinary valid retention; explicit camera deletion remains destructive. This patch does not alter those policies or claim protection against hostile concurrent filesystem changes.

## Regression scope

New tests cover partial pinning failure followed by source unlink, first-pin failure, forced queue refusal followed by buffer pruning, slot release, failed assembly with readable partial publication, validation failure, destination collision, successful assembly cleanup, canceled assembly and empty failed-capture preservation. Filesystem and hard-link operations are real. Assembly tests intentionally use fake FFmpeg/FFprobe executables and synthetic bytes, exercising process/publication/cleanup decisions rather than media validity. Cancellation tests cover already-canceled contexts, not a live FFmpeg interrupted mid-write.

The assistant has not executed these repository tests. CI results must be recorded for the resulting commit separately. Real disk/inode exhaustion, permissions and unavailable mounts, interrupted live capture/assembly, active-recorder SIGTERM, and actual FFmpeg/media validation remain integration requirements.

## Remaining bounded work

Next: versioned durable recovery manifests with stable camera/event identity, requested range, inputs, processing/failure and publication state; conservative behavior on manifest write/sync failures; read-only operator classification before any explicit recovery action. Also pending are playback failure handling, reconciliation of older lifecycle/timeline/export summaries, real-system validation and final whole-branch security/lifecycle review. Keep the audit branch unmerged. See STORAGE_ADMISSION.md, EVENT_STAGING_ADMISSION.md, STORAGE_STATUS.md and RECORDING_LIFECYCLE.md for related limits; older remaining-work summaries require reconciliation with the focused documents.
