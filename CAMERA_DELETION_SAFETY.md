# Camera deletion and retained capture evidence

Documentation prepared against f08057bdee6fb28e67c4aec56d5f3844eb323cd7 on
fix/recording-safety-audit. The original camera-deletion safety change was prepared
against 1bde998fece47d0e4e6d965c05e0804f2490cae1. This document describes scoped
behavior and authored regression coverage, not authorization to erase evidence.

## Explicit deletion policy

Single-camera and bulk-camera deletion do not recursively remove recording
directories. After recorder cancellation and join, removal permits only top-level
regular files with the .mp4 extension and empty buffer/temp directories. Published
partial MP4s are included in this published-file naming scope; playable does not
mean capture-complete. No extension-based completeness claim is made.

Any nonempty buffer/temp directory, .tmp capture, checkpoint, unexpected entry,
symlink, special file or unrecognized directory refuses that camera's removal.
Inspection of at most 8192 direct entries finishes before the first unlink. A
missing/unavailable camera directory is not considered empty: it can be lost
storage. No recursive evidence inspection, deletion, expiry or reassembly occurs.
Empty nested event directories also block removal; operators must preserve and
resolve retained evidence explicitly, outside this endpoint. There is no force
flag or automatic evidence-recovery authority implied by this policy.

A removal refusal after stop leaves the camera registered with stopped recording
intent. Some files may already have been unlinked. Review the partial outcome and
retained evidence before any explicit restart or further action. A refusal is not
proof that every file remains present.

## Batch ordering and persistence

The common engine holds recordingControlMu across the operation, never NVR.mu
across worker joins, filesystem operations or saves. Each target revalidates the
camera pointer, ID and directory. Request cancellation and shutdown are checked
before admission, after join, and between filesystem operations. A join already
started still completes: no timeout goroutine abandons live recorder ownership.
This does not make uninterruptible kernel calls or lock waits cancellable.

Bulk selection with no explicit ID list uses deterministic ID order. An explicit
selection is processed in its supplied order. Targets are processed one at a time.
A path-policy validation refusal before stop can be accumulated while processing
continues to later targets. That refusal does not change recording intent.

Once an explicit delete has stopped a recorder, its stopped intent must save
successfully before the first unlink. Even cancellation/shutdown during join does
not skip that save. Stop-save failure leaves registration intact and attempts no
file deletion for that target.

After media ownership is acquired, any removal error stops the batch: later
cameras are not processed. This includes evidence refusal during filesystem
inspection, partial unlink failure, parent/root identity refusal, completion sync
failure and failed final directory removal. The engine releases media ownership
on return. It does not unregister the failed camera or perform its final removal
save. If the stop save succeeded, that saved stopped intent remains the applicable
pre-removal state, subject to the persistence durability limits below.

Each successful directory removal is followed by registry/order removal and a
second save before another camera is processed. Persistence failures abort further
targets. After a final save failure, memory may show removal while the old durable
configuration still contains the stopped camera. Restart may therefore restore
that registration, not active recording intent. Atomic-write/directory-sync errors
can have ambiguous durability; there is no transactional deletion, rollback, or
crash-recovery guarantee.

## Filesystem identity and sync boundaries

Linux openat/O_DIRECTORY/O_NOFOLLOW pins parent and camera descriptors. Child
lookup, identity checks and unlinkat are descriptor-relative; no child directory
is recursed. The named parent is also checked without following a final symlink
and compared with the opened parent's device/inode and directory type. Parent
path disappearance, replacement, or a final symlink causes refusal at the next
checkpoint. Root identity and planned entry device/inode/mode/size/timestamps are
rechecked. Empty-directory removal uses AT_REMOVEDIR, which refuses late entries.

Each successful child unlink marks the camera directory dirty. If an error occurs
before a successful completion sync, a deferred sync is attempted while the camera
descriptor is still open. A sync failure is joined with the original error. This
attempt is not skipped merely because cancellation or a guard caused the return.

Normal completion syncs the camera directory before the final identity/guard check
and camera rmdir, then syncs the parent after rmdir. If the initial completion sync
fails after successful child unlinks, the deferred path attempts it again. Even a
successful retry does not erase the initial failure or authorize rmdir. If both
attempts fail, both errors are preserved. A successful completion sync clears the
dirty marker; a later guard/rmdir failure does not trigger a redundant deferred
camera-directory sync. Sync attempts do not restore files or prove durability when
they fail.

These checks do not provide atomic compare-and-unlink, a frozen directory, mount
identity proof, or protection against a hostile writable parent/ancestor. A late
entry or rename can cause partial deletion and failure. Trusted exclusive storage
ownership remains required; concurrent external filesystem mutation is unsupported.
Recorder joins stop this application's worker, not unrelated filesystem writers.
This document does not widen retention/orphan/published-recording cleanup policy
or certify those paths, deployment mount admission, or live storage.

## HTTP outcomes

Evidence/changed-entry refusal represented by errCameraDeleteBlocked is HTTP 409.
Other I/O errors remain failures; shutdown is HTTP 503. Persistence failure takes
precedence with HTTP 500. Missing parent/storage lookup errors are not uniformly
converted into the blocked-entry sentinel. The frontend is not changed by this
documentation batch; server responses remain authoritative for partial outcomes.

## Validation scope and evidence status

Existing authored regression groups cover evidence-byte preservation before
unlink, permitted scope, symlinks/FIFO, inspection limit, partial unlink and
cancellation, late entries, root replacement, recorder join and save/load stopped
intent, persistence failures, batch fail-stop, ordering, registry availability,
shutdown/cancellation, ownership and HTTP error precedence.

Recent focused additions are:

- camera_deletion_parent_identity_test.go: six parent-path replacement/refusal
  cases and unchanged-parent success.
- camera_deletion_partial_sync_test.go: six partial-progress/success cases.
- camera_deletion_completion_sync_test.go: five filesystem completion-boundary
  cases covering retry outcomes, post-sync refusal and late-file preservation.
- camera_deletion_completion_integration_test.go: three managed-workflow cases
  checking batch stop, registration/stopped intent, next-camera preservation,
  persistence callback sequencing and media ownership release. Its persistence
  callback captures an in-memory snapshot, not an on-disk durability result.

The user reports the baseline f08057b commit green. That report is not an
independent inspection of CI jobs or a claim that every authored test executed.
Go/gofmt/race/vet/Docker/browser or live deletion tests were not run during this
documentation preparation. Whole-branch destructive-path review, skipped-CI
reconciliation, exact-candidate consistency, live Dahua/RTSP qualification and
representative-storage acceptance remain separate outstanding gates. No merge,
deployment, cleanup/retention/recovery activation or deletion authority follows
from these fixtures or a green CI report.
