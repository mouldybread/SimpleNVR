# Camera deletion and retained capture evidence

Prepared against 1bde998fece47d0e4e6d965c05e0804f2490cae1. This is a
camera-delete safety change, not authorization to erase recovery evidence.

## Explicit deletion policy

Single-camera and bulk-camera deletion no longer recursively remove recording
directories. After recorder cancellation and join, they permit only top-level
regular files with the .mp4 extension and empty buffer/temp directories. Published
partial MP4s are included in this existing published-file naming scope; playable
does not mean capture-complete. No extension-based completeness claim is made.

Any nonempty buffer/temp directory, .tmp capture, checkpoint, unexpected entry,
symlink, special file or unrecognized directory refuses that camera's deletion.
Inspection of at most 8192 direct entries finishes before the first unlink. A
missing/unavailable camera directory is not considered empty: it can be lost
storage. No recursive evidence inspection, deletion, expiry or reassembly occurs.
Empty nested event directories also block removal; operators must preserve and
resolve retained evidence explicitly, outside this endpoint. There is no force
flag and no new recovery/cleanup interface in this batch.

A refusal after stop leaves the camera registered with stopped recording intent;
review evidence and explicitly restart recording if desired. Other bulk targets
may complete. Nonempty/evidence refusal is HTTP 409; I/O errors remain failures,
shutdown is 503, and persistence failure takes precedence with HTTP 500. The
frontend is unchanged; server responses are authoritative for partial outcomes.

## Ordering and persistence

The common engine holds recordingControlMu across the operation, never NVR.mu
across worker joins, filesystem operations or saves. Each target revalidates the
camera pointer, ID and directory. Request cancellation and shutdown are checked
before admission, after join, and between filesystem operations. A join already
started still completes: no timeout goroutine abandons live recorder ownership.
This does not make uninterruptible kernel calls or lock waits cancellable.

Once an explicit delete has stopped a recorder, its stopped intent must save
successfully before the first unlink. Even cancellation/shutdown during join does
not skip that save. Stop-save failure leaves registration intact and attempts no
file deletion for that target. A path-policy rejection before stop does not change
recording intent. Bulk targets run in deterministic ID order, one at a time.

Each successful directory removal is followed by registry/order removal and a
second save before another camera is processed. A filesystem failure leaves the
camera registered and durably stopped if its stop save succeeded. Some explicitly
selected published files may already be gone; no rollback or all-or-nothing claim
is made. Save failures abort further targets and return an error. After a final
save failure, memory may show removal while the old durable configuration still
contains the stopped camera. Restart may therefore restore that registration,
not active recording intent. Atomic-write/directory-sync errors can have ambiguous
durability; the batch does not claim transactional deletion or crash recovery.

## Filesystem boundaries

Linux openat/O_DIRECTORY/O_NOFOLLOW pins parent and camera descriptors. Lookup,
identity checks and unlinkat are descriptor-relative; no child directory is
recursed. Empty-directory removal uses AT_REMOVEDIR, which refuses late entries.
Root identity and planned entry device/inode/mode/size/timestamps are rechecked.
Directories are synced after unlinking and the parent after camera rmdir.

These checks do not provide atomic compare-and-unlink, a frozen directory, mount
identity proof, or protection against a hostile writable parent/ancestor. A late
entry or rename can cause partial deletion and failure. Trusted exclusive storage
ownership remains required; concurrent external filesystem mutation is unsupported.
Recorder joins stop this application's worker, not unrelated filesystem writers.
Existing retention/orphan/published-recording cleanup policies are not widened or
claimed comprehensively hardened here. In particular this batch does not resolve
the wider published-recording endpoint's pathname races or deployment mount gate.

## Validation scope

Authored Go regression groups cover evidence-byte preservation before unlink,
permitted published/empty-directory scope, symlinks/FIFO, inspection limit, partial
unlink/cancellation, late entries, pinned-root replacement, recorder join and actual
save/load stopped intent, pre/post deletion save failures, partial bulk order and
persistence, shutdown/cancellation during join, registry accessibility/revalidation,
and HTTP error precedence. These are tests to run in CI, not locally executed Go
evidence. No Go/gofmt/race/vet/Docker/browser candidate run is claimed during
preparation. Exact-candidate CI and runtime acceptance remain required; no merge
readiness or live RTSP/Dahua/storage deployment certification follows from this.
