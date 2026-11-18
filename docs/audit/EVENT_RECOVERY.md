# Durable event recovery checkpoints (version 1)

## Scope and schema

Each newly created event staging directory receives an owner-only recovery.json before prebuffer linking. The snapshot identifies the registered camera ID and event ID independently of camera names. It records the requested start/end and duration, event creation time, revision, checkpoint observation time, intended prebuffer basenames and metadata-derived intervals, observed successful pin count, event.mp4 capture basename, processing state/fixed failure code, and intended temporary/published output basenames with a partial flag when assembly reaches publication.

Requested time is caller intent, not a promise of acquisition at that time: current capture starts when a worker executes. Prebuffer times remain FFmpeg/filename metadata, not calibrated wall-clock coverage. A planned name does not prove a file was linked, and pinned_count is only the latest successfully checkpointed progress. Absence from a snapshot does not prove absence on disk after interruption.

The implementation caps planned prebuffer inputs at 128 and each JSON snapshot at 64 KiB. Camera/event IDs must be nonempty and at most 256 bytes; duration remains 1-3600 seconds. The manifest contains no RTSP URL, credentials, camera name/label, absolute path, or original source filename.

## Persistence and ordering

The camera directory and staging parent are synced before the initial snapshot. A replacement is written to an owner-only recovery-*.json.tmp file in the same staging directory, checked for write errors/short writes, synced, closed, atomically renamed to recovery.json, then the staging directory is synced. Successful replacement does not leave a temporary snapshot. Failed writes, syncs, closes or renames do not authorize evidence cleanup; temporary snapshots are left for inspection. A failure after rename can leave the new snapshot visible without confirmed directory-sync durability. Atomic replacement is not an append-only history or an authenticated audit log.

The pinning snapshot records intended inputs before links; pinned records completion. A link failure records pin_failed and the successful prefix count, and preserves the manifest even if no pin succeeded. capturing is persisted before launching capture; captured follows successful process completion. assembling precedes concat creation/process execution. publishing records output intent and partial status before validation/publication. published follows publication. Snapshot state may lag filesystem state after a crash; published does not establish complete/decodable footage or durable media bytes.

Any checkpoint failure latches recoveryFailed for that job. Further checkpoint attempts fail, subsequent capture/assembly/publication boundaries stop rather than proceed without the required checkpoint, and releaseJob preserves staging even if another path clears keepStaging. Failure after output publication preserves both published output and staging; the persisted snapshot can correctly remain publishing. Earlier process failures retain fixed failure codes. A retention checkpoint preserves the last processing/failure state instead of overwriting it with a generic retained state.

Successful assembly/publication without cancellation or checkpoint failure keeps the existing staging-removal behavior, including removal of its recovery snapshot. These manifests describe retained/in-flight recovery, not permanent successful-event history. Empty canceled jobs that never attempted capture may still be removed. Metadata-only events do not create manifests. HTTP event metadata persistence remains separate and nontransactional with capture admission.

## Read-only operator interpretation

Stop cleanly, preserve separate copies of configuration, event metadata and the whole affected camera directory, then inspect manifests and media together. Treat pinning/capturing/assembling/publishing as interrupted or ambiguous, not successful. Failure states identify the last observed processing failure, not the validity or recoverability of every artifact. Even published requires independent media/coverage inspection. Missing, corrupt, future-version or conflicting snapshots and identity/output mismatches require manual classification; never infer safe deletion. Retained temporary snapshots may be incomplete and are not authoritative replacements for a valid recovery.json.

No startup reader, recovery API, automatic reassembly, retries, staging expiry or deletion action is introduced. Legacy staging remains untouched and may lack identity. Do not execute retained concat lists blindly or trust manifest names as commands/paths for future recovery. Future readers must validate schema, version, bounds, identity and no-follow file identities before any action.

## Limits and verification

Sync operations strengthen manifest persistence ordering on supporting filesystems; they do not certify storage hardware, power-loss recovery or media data durability. Media files are not newly fsynced by this batch. Generated staging paths assume trusted parent directories; hostile concurrent replacement is not solved. Sync/filesystem calls are synchronous and can block while existing admission/queue/buffer locks are held. They have no interruptible syscall deadline and can delay controls/shutdown; the overall shutdown budget must still report failure rather than claim completion.

Manifest and temporary-file bytes count toward the existing bounded staging scanner. Admission thresholds remain observations, not quotas/reservations. Repeated failures can retain more snapshots/directories. No retained evidence is removed to relieve pressure. Existing explicit camera deletion and published-MP4 retention remain destructive according to their existing policies.

Regression additions cover identity/range, atomic replacement of a longer snapshot, owner-only mode, pin intent/progress on partial failure, failure latching and slot release, fail-closed capture admission, a failed published checkpoint preserving both output and inputs, manifest-size bounds and preservation of failed temporary snapshots. Assembly tests use the existing fake executables and do not validate real media. Injected writer failures model write/sync failures but do not reproduce physical disk exhaustion or power loss.

This prepared batch has not run go vet, Go tests/race tests, real FFmpeg, browser or live-camera integration. Keep the audit branch unmerged pending exact-commit CI, real-system validation and final review. See EVENT_FAILURE_PRESERVATION.md, STORAGE_ADMISSION.md, EVENT_STAGING_ADMISSION.md, STORAGE_STATUS.md and BACKGROUND_CLEANUP.md. Older remaining-work summaries still need reconciliation.
