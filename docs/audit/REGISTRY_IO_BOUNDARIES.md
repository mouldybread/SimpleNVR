# Registry locks and recording filesystem operations

## Scope at 375c580

This batch follows the user-reported green live-proxy commit 375c580. Preparation inspected lifecycle.go, playback.go, cleanup.go, handlers.go, retention_snapshot.go, private_media.go, shutdown.go, storage_admission.go, main.go and the relevant lifecycle/cleanup tests. This is a scoped source review, not final whole-branch clearance.

Storage admission previously called checkRecordingStorage/Statfs with NVR.mu held during explicit start, startup resume, mode preflight and replacement admission. The explicit published-recording delete-all handler also held NVR.mu.RLock during enumeration/stat/removal and JSON response writing. Blocking those phases could prevent registry writers and recorder shutdown coordination from acquiring the registry lock.

## Admission lock boundary

recordingControlMu still serializes starts, stops, mode changes and camera deletion. Storage checking now runs outside NVR.mu. Worker admission snapshots camera identity, directory and mode, checks storage, reacquires the registry lock, then rejects shutdown or changed state before adding to the WaitGroup and launching a worker. Changed state returns a retryable 409; missing identity remains 404. A loaded true intent without a runtime cancel remains restart intent.

Mode preflight still rejects known storage failures before stopping the old worker. The replacement has a second admission check after the old worker joins. This is still an observation, not a quota/reservation. A normal second-check failure leaves the selected new mode inactive, as before. If shutdown intervenes during replacement admission, restore the previous recording intent and never launch a late worker. Depending on phase, the mode selection may already have changed. HTTP mutation joining must still precede final persistence.

## Explicit published-recording deletion

The handler takes recordingControlMu, briefly snapshots registered directory strings under NVR.mu.RLock, and performs filesystem work and JSON encoding after releasing NVR.mu. Cameras added after the snapshot are not part of that request. Existing camera-add serialization gaps and broad destructive-path review are not declared solved.

The deletion pass reads the directory with os.ReadDir so missing/unreadable directories are reported instead of being hidden by Glob's I/O-error behavior. Only top-level regular .mp4 entries are eligible. Lstat skips stable symlinks, directories and special entries. .tmp capture outputs, nested temp/event manifests/inputs and buffer contents are not traversed. Published _partial.mp4 files remain eligible for this existing explicit destructive API; this is not authority to delete staging evidence or reassemble anything.

The existing JSON success/deleted/size_mb fields remain, with failures added. HTTP 200 can describe partial completion; callers must inspect success. Only successful unlinks contribute bytes/count. Entries concurrently absent are not counted as failures. Cancellation checks occur between filesystem operations, and shutdown is checked before admission and between camera directories. Already completed removals are not rolled back.

## Tests and remaining limits

Per-call callbacks, never mutable global hooks, let tests pause all admission phases, prove registry readers/writers can proceed, run recorder cancellation/joining, and assert no new worker launches after shutdown. Tests cover state/identity revalidation, preflight versus second-check failure and restart-intent preservation. Deletion tests cover blocked callback registry access, partial JSON accounting, real temporary-directory preservation/missing-directory fixtures, injected unlink permission errors, cancellation and method/shutdown refusal.

No new Go test, build, vet or race result is claimed: Go/gofmt are unavailable locally. Source blob verification and exact patch apply/reverse checks are preparation evidence only. The paused callbacks are not actual blocked Statfs or lost-mount tests. Existing concurrency/lifecycle tests remain mandatory in CI, along with exact-candidate validation.

This does not make filesystem syscalls interruptible. recordingControlMu still spans admission/deletion filesystem work, retentionPolicyMu still spans retention deletion, and shutdown may time out waiting for an uninterruptible mutation. Directory enumeration is not newly resource bounded. Pathname-based deletion still permits parent/leaf replacement races; stable-symlink tests are not race elimination. Orphan recovery inspection/deletion, recursive camera deletion, persistence/recovery semantics, mount-loss behavior and final whole-branch review remain open. The private-media descriptor handling and live-proxy response policy are unchanged. No merge or repository write is authorized by preparation.
